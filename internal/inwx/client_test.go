package inwx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/k2b-dev/inwx-cli/internal/dns"
)

func TestLoginAndTwoFactorUnlock(t *testing.T) {
	t.Parallel()

	var methods []string
	var mutex sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var call struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Error(err)
			return
		}
		mutex.Lock()
		methods = append(methods, call.Method)
		mutex.Unlock()

		switch call.Method {
		case "account.login":
			writeEnvelope(t, writer, 1000, map[string]string{"tfa": "GOOGLE-AUTH"})
		case "account.unlock":
			var params struct {
				TAN string `json:"tan"`
			}
			if err := json.Unmarshal(call.Params, &params); err != nil {
				t.Error(err)
			}
			if params.TAN != "287082" {
				t.Errorf("got TAN %q, want RFC vector", params.TAN)
			}
			writeEnvelope(t, writer, 1000, struct{}{})
		default:
			t.Errorf("unexpected method %q", call.Method)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, Options{
		SharedSecret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
		Now:          func() time.Time { return time.Unix(59, 0) },
	})
	if err := client.Login(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(methods) != "[account.login account.unlock]" {
		t.Fatalf("unexpected calls %v", methods)
	}
}

func TestLoginClassifiesInvalidCredentials(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, writer, 2200, struct{}{})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, Options{})
	err := client.Login(context.Background())
	var auth *AuthError
	var api *APIError
	if !errors.As(err, &auth) || !errors.As(err, &api) || api.Code != 2200 {
		t.Fatalf("unexpected error %T %v", err, err)
	}
}

func TestLoginRejectsUnknownTwoFactorMethod(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, writer, 1000, map[string]string{"tfa": "SMS"})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, Options{})
	err := client.Login(context.Background())
	var auth *AuthError
	if !errors.As(err, &auth) || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unexpected error %T %v", err, err)
	}
}

func TestUnlockErrorRedactsTOTPCode(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var call struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Error(err)
			return
		}
		if call.Method == "account.login" {
			writeEnvelope(t, writer, 1000, map[string]string{"tfa": "GOOGLE-AUTH"})
			return
		}
		if err := json.NewEncoder(writer).Encode(map[string]any{
			"code":    2200,
			"msg":     "rejected 287082",
			"resData": map[string]any{},
		}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, Options{
		SharedSecret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
		Now:          func() time.Time { return time.Unix(59, 0) },
	})
	err := client.Login(context.Background())
	if err == nil ||
		strings.Contains(err.Error(), "287082") ||
		!strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("TOTP was not redacted: %v", err)
	}
}

func TestListZonesPaginatesAndSorts(t *testing.T) {
	t.Parallel()

	var page atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		current := page.Add(1)
		fixture := "nameserver-list-page-1.json"
		if current == 2 {
			fixture = "nameserver-list-page-2.json"
		}
		content, err := os.ReadFile(filepath.Join("testdata", fixture))
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = writer.Write(content)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, Options{})
	zones, err := client.ListZones(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if page.Load() != 2 || len(zones) != 2 {
		t.Fatalf("unexpected pagination: calls=%d zones=%d", page.Load(), len(zones))
	}
	if zones[0].Name != "example.test." || zones[1].Name != "xn--bcher-kva.test." {
		t.Fatalf("zones not normalized and sorted: %#v", zones)
	}
}

func TestListZonesTrimsProviderWhitespace(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, writer, 1000, map[string]any{
			"count": 1,
			"domains": []map[string]any{{
				"domain": "rz.it.kolb-antik.com ",
				"type":   "MASTER",
			}},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, Options{})
	zones, err := client.ListZones(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(zones) != 1 || zones[0].Name != "rz.it.kolb-antik.com." {
		t.Fatalf("unexpected zones %#v", zones)
	}
}

func TestListRecordsUsesCompleteFixture(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join("testdata", "nameserver-info-records.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(content)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, Options{})
	records, err := client.ListRecords(context.Background(), "example.test.")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 5 {
		t.Fatalf("got %d records, want 5", len(records))
	}
	for _, record := range records {
		if record.ID == "" {
			t.Fatal("record ID was discarded")
		}
	}
}

func TestListRecordsPreservesNonCanonicalProviderTarget(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, writer, 1000, map[string]any{
			"count": 1,
			"record": []map[string]any{{
				"id":      "81006",
				"name":    "gw.it.kolb-antik.com",
				"type":    "CNAME",
				"content": "109_70_197_43.rz.it.kolb-antik.com",
				"ttl":     300,
				"prio":    0,
			}},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, Options{})
	records, err := client.ListRecords(context.Background(), "kolb-antik.com.")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 ||
		records[0].ID != "81006" ||
		records[0].Value != "109_70_197_43.rz.it.kolb-antik.com." {
		t.Fatalf("unexpected records %#v", records)
	}
}

func TestMalformedResponseIsNotRetried(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = writer.Write([]byte("{"))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, Options{Retries: 2})
	err := client.call(context.Background(), "nameserver.list", struct{}{}, nil, true)
	if err == nil {
		t.Fatal("expected malformed response error")
	}
	if calls.Load() != 1 {
		t.Fatalf("malformed response retried %d times", calls.Load())
	}
}

func TestRetryableHTTPStatusRetries(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	var delays []time.Duration
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeEnvelope(t, writer, 1000, struct{}{})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, Options{
		Retries: 1,
		Sleep: func(_ context.Context, duration time.Duration) error {
			delays = append(delays, duration)
			return nil
		},
	})
	if err := client.call(context.Background(), "nameserver.list", struct{}{}, nil, true); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || fmt.Sprint(delays) != "[250ms]" {
		t.Fatalf("unexpected retry calls=%d delays=%v", calls.Load(), delays)
	}
}

func TestRedirectIsRejected(t *testing.T) {
	t.Parallel()

	destinationCalls := atomic.Int32{}
	destination := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		destinationCalls.Add(1)
		writeEnvelope(t, writer, 1000, struct{}{})
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	client := newTestClient(t, source.URL, Options{})
	err := client.call(context.Background(), "account.login", struct{}{}, nil, false)
	if err == nil || destinationCalls.Load() != 0 {
		t.Fatalf("redirect was followed: err=%v calls=%d", err, destinationCalls.Load())
	}
}

func TestRequestHonorsContextTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
		case <-time.After(200 * time.Millisecond):
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, Options{Retries: 2})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := client.call(ctx, "nameserver.list", struct{}{}, nil, true)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want context deadline", err)
	}
}

func TestRecordCountMismatchFailsClosed(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeEnvelope(t, writer, 1000, map[string]any{
			"count":  1,
			"record": []any{},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, Options{})
	if _, err := client.ListRecords(context.Background(), "example.test."); err == nil {
		t.Fatal("expected partial response error")
	}
}

func TestMutationMethodsUseExactParametersAndNeverRetry(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		var call struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Error(err)
			return
		}
		if call.Method == "nameserver.createRecord" {
			if call.Params["domain"] != "example.test" ||
				call.Params["name"] != "www.example.test" ||
				call.Params["content"] != "192.0.2.1" ||
				call.Params["type"] != "A" {
				t.Errorf("unexpected create params %#v", call.Params)
			}
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeEnvelope(t, writer, 1000, struct{}{})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, Options{Retries: 5})
	record := dns.Record{
		Zone:  "example.test.",
		Name:  "www",
		FQDN:  "www.example.test.",
		Type:  "A",
		Value: "192.0.2.1",
		TTL:   3600,
	}
	if _, err := client.CreateRecord(context.Background(), record); err == nil {
		t.Fatal("expected create transport error")
	}
	if calls.Load() != 1 {
		t.Fatalf("mutation retried %d times", calls.Load())
	}
}

func newTestClient(t *testing.T, endpoint string, override Options) *Client {
	t.Helper()
	override.Endpoint = endpoint
	override.Username = "operator"
	override.Password = "credential"
	if override.HTTPClient == nil {
		override.HTTPClient = &http.Client{}
	}
	client, err := New(override)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func writeEnvelope(t *testing.T, writer http.ResponseWriter, code int, data any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{
		"code":    code,
		"msg":     "test response",
		"resData": data,
	}); err != nil {
		t.Error(err)
	}
}
