package inwx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"strings"
	"time"

	"github.com/k2b-dev/inwx-cli/internal/dns"
)

const (
	maxResponseSize = 4 << 20
	zonePageLimit   = 100
	maxZones        = 10000
)

type Options struct {
	Endpoint     string
	Username     string
	Password     string
	SharedSecret string
	Retries      int
	HTTPClient   *http.Client
	Now          func() time.Time
	Sleep        func(context.Context, time.Duration) error
}

type Client struct {
	endpoint     string
	username     string
	password     string
	sharedSecret string
	retries      int
	httpClient   *http.Client
	now          func() time.Time
	sleep        func(context.Context, time.Duration) error
}

type APIError struct {
	Code       int
	Message    string
	ReasonCode string
	Reason     string
}

func (err *APIError) Error() string {
	message := fmt.Sprintf("INWX API code %d", err.Code)
	if err.Message != "" {
		message += ": " + err.Message
	}
	if err.Reason != "" {
		message += ": " + err.Reason
	}
	return message
}

type AuthError struct {
	Err error
}

func (err *AuthError) Error() string {
	return "authentication failed: " + err.Err.Error()
}

func (err *AuthError) Unwrap() error {
	return err.Err
}

type responseEnvelope struct {
	Code       int             `json:"code"`
	Message    string          `json:"msg"`
	ReasonCode string          `json:"reasonCode"`
	Reason     string          `json:"reason"`
	Data       json.RawMessage `json:"resData"`
}

type Zone struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func New(options Options) (*Client, error) {
	if options.Endpoint == "" {
		return nil, errors.New("INWX endpoint is required")
	}
	if options.Retries < 0 || options.Retries > 5 {
		return nil, errors.New("retries must be between 0 and 5")
	}

	httpClient := options.HTTPClient
	if httpClient == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, fmt.Errorf("create cookie jar: %w", err)
		}
		httpClient = &http.Client{
			Jar: jar,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return errors.New("INWX endpoint redirects are not allowed")
			},
		}
	} else {
		copy := *httpClient
		if copy.Jar == nil {
			jar, err := cookiejar.New(nil)
			if err != nil {
				return nil, fmt.Errorf("create cookie jar: %w", err)
			}
			copy.Jar = jar
		}
		if copy.CheckRedirect == nil {
			copy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
				return errors.New("INWX endpoint redirects are not allowed")
			}
		}
		httpClient = &copy
	}

	now := options.Now
	if now == nil {
		now = time.Now
	}
	sleep := options.Sleep
	if sleep == nil {
		sleep = sleepContext
	}

	return &Client{
		endpoint:     options.Endpoint,
		username:     options.Username,
		password:     options.Password,
		sharedSecret: options.SharedSecret,
		retries:      options.Retries,
		httpClient:   httpClient,
		now:          now,
		sleep:        sleep,
	}, nil
}

func (client *Client) Login(ctx context.Context) error {
	var data struct {
		TFA string `json:"tfa"`
	}
	if err := client.call(ctx, "account.login", map[string]string{
		"user": client.username,
		"pass": client.password,
	}, &data, true); err != nil {
		return &AuthError{Err: err}
	}

	switch data.TFA {
	case "", "0":
		return nil
	case "GOOGLE-AUTH":
	default:
		return &AuthError{Err: fmt.Errorf("unsupported INWX two-factor method %q", data.TFA)}
	}

	code, err := generateTOTP(client.sharedSecret, client.now())
	if err != nil {
		return &AuthError{Err: err}
	}
	if err := client.call(ctx, "account.unlock", map[string]string{
		"tan": code,
	}, nil, true); err != nil {
		return &AuthError{Err: redactError(err, code)}
	}
	return nil
}

func (client *Client) Logout(ctx context.Context) error {
	return client.call(ctx, "account.logout", struct{}{}, nil, false)
}

func (client *Client) ListZones(ctx context.Context) ([]Zone, error) {
	zones := make([]Zone, 0)
	seen := make(map[string]struct{})
	total := -1

	for page := 1; ; page++ {
		var data struct {
			Count   int `json:"count"`
			Domains []struct {
				Domain string `json:"domain"`
				Type   string `json:"type"`
			} `json:"domains"`
		}
		if err := client.call(ctx, "nameserver.list", map[string]int{
			"page":      page,
			"pagelimit": zonePageLimit,
		}, &data, true); err != nil {
			return nil, err
		}

		if data.Count < 0 || data.Count > maxZones {
			return nil, fmt.Errorf("INWX returned invalid zone count %d", data.Count)
		}
		if total == -1 {
			total = data.Count
		} else if data.Count != total {
			return nil, fmt.Errorf("INWX zone count changed during pagination")
		}
		if len(data.Domains) == 0 && len(zones) < total {
			return nil, errors.New("INWX returned an empty zone page before pagination completed")
		}

		for _, item := range data.Domains {
			zone, err := dns.NormalizeZone(strings.TrimSpace(item.Domain))
			if err != nil {
				return nil, fmt.Errorf("normalize INWX zone %q: %w", item.Domain, err)
			}
			if _, exists := seen[zone]; exists {
				return nil, fmt.Errorf("INWX returned duplicate zone %q", zone)
			}
			seen[zone] = struct{}{}
			zones = append(zones, Zone{Name: zone, Type: item.Type})
		}

		if len(zones) >= total {
			if len(zones) != total {
				return nil, fmt.Errorf("INWX zone count does not match returned zones")
			}
			break
		}
	}

	sortZones(zones)
	return zones, nil
}

func (client *Client) ListRecords(ctx context.Context, zone string) ([]dns.Record, error) {
	var data struct {
		Count   int             `json:"count"`
		Records []dns.RawRecord `json:"record"`
	}
	if err := client.call(ctx, "nameserver.info", map[string]string{
		"domain": strings.TrimSuffix(zone, "."),
	}, &data, true); err != nil {
		return nil, err
	}
	if data.Count != len(data.Records) {
		return nil, fmt.Errorf(
			"INWX record count %d does not match returned records %d",
			data.Count,
			len(data.Records),
		)
	}

	records := make([]dns.Record, 0, len(data.Records))
	seen := make(map[string]struct{})
	for _, raw := range data.Records {
		record, err := dns.FromAPI(zone, raw)
		if err != nil {
			return nil, fmt.Errorf("normalize INWX record: %w", err)
		}
		if _, exists := seen[record.ID]; exists {
			return nil, fmt.Errorf("INWX returned duplicate record ID %q", record.ID)
		}
		seen[record.ID] = struct{}{}
		records = append(records, record)
	}
	dns.SortRecords(records)
	return records, nil
}

func (client *Client) CreateRecord(ctx context.Context, record dns.Record) (string, error) {
	var data struct {
		ID dns.StringID `json:"id"`
	}
	if err := client.call(ctx, "nameserver.createRecord", recordParams(record, true), &data, false); err != nil {
		return "", err
	}
	if data.ID == "" {
		return "", errors.New("INWX create response is missing the record ID")
	}
	return string(data.ID), nil
}

func (client *Client) UpdateRecord(ctx context.Context, record dns.Record) error {
	id, err := apiRecordID(record.ID)
	if err != nil {
		return err
	}
	params := recordParams(record, false)
	params["id"] = id
	return client.call(ctx, "nameserver.updateRecord", params, nil, false)
}

func (client *Client) DeleteRecord(ctx context.Context, id string) error {
	apiID, err := apiRecordID(id)
	if err != nil {
		return err
	}
	return client.call(ctx, "nameserver.deleteRecord", map[string]uint64{"id": apiID}, nil, false)
}

func recordParams(record dns.Record, includeDomain bool) map[string]any {
	content := record.Value
	if record.Type == "CNAME" || record.Type == "MX" {
		content = strings.TrimSuffix(content, ".")
	}
	priority := uint16(0)
	if record.Priority != nil {
		priority = *record.Priority
	}
	params := map[string]any{
		"name":    strings.TrimSuffix(record.FQDN, "."),
		"type":    record.Type,
		"content": content,
		"ttl":     record.TTL,
		"prio":    priority,
	}
	if includeDomain {
		params["domain"] = strings.TrimSuffix(record.Zone, ".")
	}
	return params
}

func apiRecordID(id string) (uint64, error) {
	value, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return 0, errors.New("record ID returned by INWX is not an unsigned integer")
	}
	return value, nil
}

func (client *Client) call(
	ctx context.Context,
	method string,
	params any,
	target any,
	retry bool,
) error {
	body, err := json.Marshal(map[string]any{
		"method": method,
		"params": params,
	})
	if err != nil {
		return fmt.Errorf("encode INWX request: %w", err)
	}

	attempts := 1
	if retry {
		attempts += client.retries
	}
	for attempt := 0; attempt < attempts; attempt++ {
		envelope, retryAfter, err := client.do(ctx, body)
		if err == nil {
			if envelope.Code < 1000 || envelope.Code > 1500 {
				return &APIError{
					Code:       envelope.Code,
					Message:    envelope.Message,
					ReasonCode: envelope.ReasonCode,
					Reason:     envelope.Reason,
				}
			}
			if target == nil || len(envelope.Data) == 0 || bytes.Equal(envelope.Data, []byte("null")) {
				return nil
			}
			if err := json.Unmarshal(envelope.Data, target); err != nil {
				return fmt.Errorf("decode INWX response data: %w", err)
			}
			return nil
		}

		if attempt+1 == attempts || !isRetryable(err) {
			return err
		}
		delay := retryDelay(attempt)
		if retryAfter > delay {
			delay = retryAfter
		}
		if err := client.sleep(ctx, delay); err != nil {
			return err
		}
	}
	return errors.New("INWX request attempts exhausted")
}

type httpStatusError struct {
	Status int
}

func (err *httpStatusError) Error() string {
	return fmt.Sprintf("INWX HTTP status %d", err.Status)
}

type transportError struct {
	Err error
}

func (err *transportError) Error() string {
	return "send INWX request: " + err.Err.Error()
}

func (err *transportError) Unwrap() error {
	return err.Err
}

func (client *Client) do(
	ctx context.Context,
	body []byte,
) (responseEnvelope, time.Duration, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		client.endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return responseEnvelope{}, 0, fmt.Errorf("create INWX request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	response, err := client.httpClient.Do(request)
	if err != nil {
		return responseEnvelope{}, 0, &transportError{Err: err}
	}
	defer response.Body.Close()

	retryAfter := parseRetryAfter(response.Header.Get("Retry-After"))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return responseEnvelope{}, retryAfter, &httpStatusError{Status: response.StatusCode}
	}

	content, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil {
		return responseEnvelope{}, 0, fmt.Errorf("read INWX response: %w", err)
	}
	if len(content) > maxResponseSize {
		return responseEnvelope{}, 0, errors.New("INWX response exceeds 4 MiB")
	}

	var envelope responseEnvelope
	if err := json.Unmarshal(content, &envelope); err != nil {
		return responseEnvelope{}, 0, fmt.Errorf("decode INWX response: %w", err)
	}
	if envelope.Code == 0 {
		return responseEnvelope{}, 0, errors.New("INWX response is missing a code")
	}
	return envelope, 0, nil
}

func isRetryable(err error) bool {
	var status *httpStatusError
	if errors.As(err, &status) {
		switch status.Status {
		case http.StatusTooManyRequests, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}
	var transport *transportError
	return errors.As(err, &transport) &&
		!errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded)
}

func retryDelay(attempt int) time.Duration {
	switch attempt {
	case 0:
		return 250 * time.Millisecond
	case 1:
		return time.Second
	default:
		return 2 * time.Second
	}
}

func parseRetryAfter(value string) time.Duration {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds < 0 || seconds > 300 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func redactError(err error, secret string) error {
	var api *APIError
	if errors.As(err, &api) {
		copy := *api
		copy.Message = strings.ReplaceAll(copy.Message, secret, "[REDACTED]")
		copy.Reason = strings.ReplaceAll(copy.Reason, secret, "[REDACTED]")
		return &copy
	}
	return errors.New(strings.ReplaceAll(err.Error(), secret, "[REDACTED]"))
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func sortZones(zones []Zone) {
	for index := 1; index < len(zones); index++ {
		for current := index; current > 0; current-- {
			left, right := zones[current-1], zones[current]
			if left.Name < right.Name || (left.Name == right.Name && left.Type <= right.Type) {
				break
			}
			zones[current-1], zones[current] = right, left
		}
	}
}
