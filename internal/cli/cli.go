package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/k2b-dev/inwx-cli/internal/config"
	"github.com/k2b-dev/inwx-cli/internal/dns"
	"github.com/k2b-dev/inwx-cli/internal/inwx"
)

const schemaVersion = "inwx.cli/v1"

type Options struct {
	Version          string
	Commit           string
	Date             string
	LookupEnv        config.LookupEnv
	HTTPClient       *http.Client
	EndpointOverride string
	Now              func() time.Time
	Sleep            func(context.Context, time.Duration) error
}

type globalOptions struct {
	json        bool
	environment string
	timeout     time.Duration
	retries     int
}

type commandError struct {
	Code       int
	Kind       string
	Message    string
	APICode    int
	ReasonCode string
}

func (err *commandError) Error() string {
	return err.Message
}

func Run(
	parent context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	options Options,
) int {
	if options.LookupEnv == nil {
		options.LookupEnv = os.LookupEnv
	}

	global, rest, err := parseGlobal(args)
	if err != nil {
		return writeFailure(stderr, global.json, "", environmentLabel(global, options), usageError(err))
	}

	environment, err := config.ResolveEnvironment(global.environment, options.LookupEnv)
	if err != nil {
		return writeFailure(stderr, global.json, "", environmentLabel(global, options), configError(err))
	}
	if options.EndpointOverride != "" {
		environment.Endpoint = options.EndpointOverride
	}

	if len(rest) == 0 {
		writeRootHelp(stdout)
		return 0
	}

	command := commandName(rest)
	ctx, cancel := context.WithTimeout(parent, global.timeout)
	defer cancel()

	var runErr *commandError
	switch rest[0] {
	case "help":
		if len(rest) != 1 {
			runErr = usageError(errors.New("help accepts no arguments"))
		} else {
			writeRootHelp(stdout)
		}
	case "version":
		runErr = runVersion(rest[1:], stdout, global, environment, options)
	case "auth":
		runErr = runAuth(ctx, rest[1:], stdout, global, environment, options)
	case "dns":
		runErr = runDNS(ctx, rest[1:], stdout, global, environment, options)
	default:
		runErr = usageError(fmt.Errorf("unknown command %q", rest[0]))
	}
	if runErr == nil {
		return 0
	}
	if errors.Is(parent.Err(), context.Canceled) {
		runErr.Code = 130
		runErr.Kind = "interrupted"
		runErr.Message = "interrupted"
	}
	return writeFailure(stderr, global.json, command, environment.Name, runErr)
}

func parseGlobal(args []string) (globalOptions, []string, error) {
	options := globalOptions{
		timeout: 20 * time.Second,
		retries: 2,
	}
	flags := flag.NewFlagSet("inwx", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(&options.json, "json", false, "")
	flags.StringVar(&options.environment, "environment", "", "")
	flags.DurationVar(&options.timeout, "timeout", options.timeout, "")
	flags.IntVar(&options.retries, "retries", options.retries, "")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options, []string{"help"}, nil
		}
		return options, nil, err
	}
	if options.timeout < time.Second || options.timeout > 5*time.Minute {
		return options, nil, errors.New("--timeout must be between 1s and 5m")
	}
	if options.retries < 0 || options.retries > 5 {
		return options, nil, errors.New("--retries must be between 0 and 5")
	}
	return options, flags.Args(), nil
}

func runVersion(
	args []string,
	stdout io.Writer,
	global globalOptions,
	environment config.Environment,
	options Options,
) *commandError {
	if len(args) == 1 && isHelp(args[0]) {
		_, _ = fmt.Fprintln(stdout, "Usage: inwx [global flags] version")
		return nil
	}
	if len(args) != 0 {
		return usageError(errors.New("version accepts no arguments"))
	}
	data := struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
		Date    string `json:"date"`
	}{
		Version: options.Version,
		Commit:  options.Commit,
		Date:    options.Date,
	}
	if global.json {
		return writeSuccess(stdout, "version", environment.Name, data)
	}
	_, _ = fmt.Fprintf(stdout, "inwx %s (commit %s, built %s)\n", data.Version, data.Commit, data.Date)
	return nil
}

func runAuth(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	global globalOptions,
	environment config.Environment,
	options Options,
) *commandError {
	if len(args) == 1 && isHelp(args[0]) {
		_, _ = fmt.Fprintln(stdout, "Usage: inwx [global flags] auth check")
		return nil
	}
	if len(args) == 2 && args[0] == "check" && isHelp(args[1]) {
		_, _ = fmt.Fprintln(stdout, "Usage: inwx [global flags] auth check")
		return nil
	}
	if len(args) != 1 || args[0] != "check" {
		return usageError(errors.New("usage: inwx auth check"))
	}

	client, credentials, err := authenticatedClient(ctx, global, environment, options)
	if err != nil {
		return classify(err, credentials)
	}
	defer logout(client)

	data := struct {
		Authenticated bool `json:"authenticated"`
	}{Authenticated: true}
	if global.json {
		return writeSuccess(stdout, "auth.check", environment.Name, data)
	}
	_, _ = fmt.Fprintf(stdout, "Authentication successful (%s)\n", environment.Name)
	return nil
}

func runDNS(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	global globalOptions,
	environment config.Environment,
	options Options,
) *commandError {
	if len(args) == 1 && isHelp(args[0]) {
		_, _ = io.WriteString(stdout, `Usage:
  inwx [global flags] dns zones list
  inwx [global flags] dns records list <zone> [--type TYPE] [--name NAME]
`)
		return nil
	}
	if len(args) < 2 {
		return usageError(errors.New("usage: inwx dns zones list | inwx dns records list <zone>"))
	}
	switch args[0] {
	case "zones":
		if len(args) == 2 && isHelp(args[1]) {
			_, _ = fmt.Fprintln(stdout, "Usage: inwx [global flags] dns zones list")
			return nil
		}
		if len(args) == 3 && args[1] == "list" && isHelp(args[2]) {
			_, _ = fmt.Fprintln(stdout, "Usage: inwx [global flags] dns zones list")
			return nil
		}
		if len(args) != 2 || args[1] != "list" {
			return usageError(errors.New("usage: inwx dns zones list"))
		}
		return runZones(ctx, stdout, global, environment, options)
	case "records":
		if len(args) == 2 && isHelp(args[1]) {
			_, _ = fmt.Fprintln(
				stdout,
				"Usage: inwx [global flags] dns records list <zone> [--type TYPE] [--name NAME]",
			)
			return nil
		}
		if len(args) == 3 && args[1] == "list" && isHelp(args[2]) {
			_, _ = fmt.Fprintln(
				stdout,
				"Usage: inwx [global flags] dns records list <zone> [--type TYPE] [--name NAME]",
			)
			return nil
		}
		if args[1] != "list" {
			return usageError(errors.New("usage: inwx dns records list <zone>"))
		}
		return runRecords(ctx, args[2:], stdout, global, environment, options)
	default:
		return usageError(fmt.Errorf("unknown dns command %q", args[0]))
	}
}

func runZones(
	ctx context.Context,
	stdout io.Writer,
	global globalOptions,
	environment config.Environment,
	options Options,
) *commandError {
	client, credentials, err := authenticatedClient(ctx, global, environment, options)
	if err != nil {
		return classify(err, credentials)
	}
	defer logout(client)

	zones, err := client.ListZones(ctx)
	if err != nil {
		return classify(err, credentials)
	}
	data := struct {
		Zones []inwx.Zone `json:"zones"`
	}{Zones: zones}
	if global.json {
		return writeSuccess(stdout, "dns.zones.list", environment.Name, data)
	}
	if len(zones) == 0 {
		_, _ = fmt.Fprintln(stdout, "No zones found.")
		return nil
	}
	table := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "ZONE\tTYPE")
	for _, zone := range zones {
		_, _ = fmt.Fprintf(table, "%s\t%s\n", zone.Name, zone.Type)
	}
	_ = table.Flush()
	return nil
}

func runRecords(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	global globalOptions,
	environment config.Environment,
	options Options,
) *commandError {
	if len(args) == 0 {
		return usageError(errors.New("usage: inwx dns records list <zone> [--type TYPE] [--name NAME]"))
	}
	if len(args) == 2 && isHelp(args[1]) {
		_, _ = fmt.Fprintln(
			stdout,
			"Usage: inwx [global flags] dns records list <zone> [--type TYPE] [--name NAME]",
		)
		return nil
	}
	zone, err := dns.NormalizeZone(args[0])
	if err != nil {
		return usageError(err)
	}

	flags := flag.NewFlagSet("records list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var typeFilter, nameFilter string
	flags.StringVar(&typeFilter, "type", "", "")
	flags.StringVar(&nameFilter, "name", "", "")
	if err := flags.Parse(args[1:]); err != nil {
		return usageError(err)
	}
	if flags.NArg() != 0 {
		return usageError(errors.New("records list received unexpected arguments"))
	}
	typeFilter = strings.ToUpper(typeFilter)
	if typeFilter != "" && !supportedFilterType(typeFilter) {
		return usageError(errors.New("--type must be A, AAAA, CNAME, TXT, or MX"))
	}

	canonicalName := ""
	if nameFilter != "" {
		canonicalName, _, err = dns.NormalizeInputName(nameFilter, zone)
		if err != nil {
			return usageError(err)
		}
	}

	client, credentials, clientErr := authenticatedClient(ctx, global, environment, options)
	if clientErr != nil {
		return classify(clientErr, credentials)
	}
	defer logout(client)

	records, clientErr := client.ListRecords(ctx, zone)
	if clientErr != nil {
		return classify(clientErr, credentials)
	}
	filtered := records[:0]
	for _, record := range records {
		if typeFilter != "" && record.Type != typeFilter {
			continue
		}
		if canonicalName != "" && record.Name != canonicalName {
			continue
		}
		filtered = append(filtered, record)
	}

	data := struct {
		Zone    string       `json:"zone"`
		Records []dns.Record `json:"records"`
	}{Zone: zone, Records: filtered}
	if global.json {
		return writeSuccess(stdout, "dns.records.list", environment.Name, data)
	}
	if len(filtered) == 0 {
		_, _ = fmt.Fprintf(stdout, "No records found in %s.\n", zone)
		return nil
	}
	table := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "ID\tNAME\tTYPE\tVALUE\tTTL\tPRIORITY")
	for _, record := range filtered {
		priority := "-"
		if record.Priority != nil {
			priority = strconv.Itoa(int(*record.Priority))
		}
		_, _ = fmt.Fprintf(
			table,
			"%s\t%s\t%s\t%s\t%d\t%s\n",
			record.ID,
			record.Name,
			record.Type,
			record.Value,
			record.TTL,
			priority,
		)
	}
	_ = table.Flush()
	return nil
}

func authenticatedClient(
	ctx context.Context,
	global globalOptions,
	environment config.Environment,
	options Options,
) (*inwx.Client, config.Credentials, error) {
	credentials, err := config.LoadCredentials(options.LookupEnv)
	if err != nil {
		return nil, credentials, err
	}
	client, err := inwx.New(inwx.Options{
		Endpoint:     environment.Endpoint,
		Username:     credentials.Username,
		Password:     credentials.Password,
		SharedSecret: credentials.SharedSecret,
		Retries:      global.retries,
		HTTPClient:   options.HTTPClient,
		Now:          options.Now,
		Sleep:        options.Sleep,
	})
	if err != nil {
		return nil, credentials, err
	}
	if err := client.Login(ctx); err != nil {
		return nil, credentials, err
	}
	return client, credentials, nil
}

func classify(err error, credentials config.Credentials) *commandError {
	message := redact(err.Error(), credentials)
	var auth *inwx.AuthError
	if errors.As(err, &auth) {
		result := &commandError{Code: 3, Kind: "authentication", Message: message}
		var api *inwx.APIError
		if errors.As(err, &api) {
			result.APICode = api.Code
			result.ReasonCode = api.ReasonCode
		}
		return result
	}
	var api *inwx.APIError
	if errors.As(err, &api) {
		return &commandError{
			Code:       4,
			Kind:       "api",
			Message:    message,
			APICode:    api.Code,
			ReasonCode: api.ReasonCode,
		}
	}
	if strings.Contains(message, "INWX_") {
		return configError(errors.New(message))
	}
	return &commandError{Code: 4, Kind: "api", Message: message}
}

func redact(message string, credentials config.Credentials) string {
	for _, secret := range []string{
		credentials.Username,
		credentials.Password,
		credentials.SharedSecret,
	} {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
		}
	}
	return message
}

func logout(client *inwx.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = client.Logout(ctx)
}

func supportedFilterType(value string) bool {
	switch value {
	case "A", "AAAA", "CNAME", "TXT", "MX":
		return true
	default:
		return false
	}
}

func isHelp(value string) bool {
	return value == "--help" || value == "-h"
}

func writeSuccess(
	writer io.Writer,
	command string,
	environment string,
	data any,
) *commandError {
	envelope := struct {
		SchemaVersion string `json:"schema_version"`
		Command       string `json:"command"`
		Environment   string `json:"environment"`
		OK            bool   `json:"ok"`
		Data          any    `json:"data"`
	}{
		SchemaVersion: schemaVersion,
		Command:       command,
		Environment:   environment,
		OK:            true,
		Data:          data,
	}
	if err := json.NewEncoder(writer).Encode(envelope); err != nil {
		return &commandError{Code: 4, Kind: "output", Message: err.Error()}
	}
	return nil
}

func writeFailure(
	writer io.Writer,
	jsonMode bool,
	command string,
	environment string,
	err *commandError,
) int {
	if !jsonMode {
		_, _ = fmt.Fprintf(writer, "inwx: %s\n", err.Message)
		return err.Code
	}
	errorData := struct {
		Kind       string `json:"kind"`
		Message    string `json:"message"`
		APICode    int    `json:"api_code,omitempty"`
		ReasonCode string `json:"reason_code,omitempty"`
	}{
		Kind:       err.Kind,
		Message:    err.Message,
		APICode:    err.APICode,
		ReasonCode: err.ReasonCode,
	}
	envelope := struct {
		SchemaVersion string `json:"schema_version"`
		Command       string `json:"command"`
		Environment   string `json:"environment"`
		OK            bool   `json:"ok"`
		Error         any    `json:"error"`
	}{
		SchemaVersion: schemaVersion,
		Command:       command,
		Environment:   environment,
		OK:            false,
		Error:         errorData,
	}
	_ = json.NewEncoder(writer).Encode(envelope)
	return err.Code
}

func usageError(err error) *commandError {
	return &commandError{Code: 2, Kind: "usage", Message: err.Error()}
}

func configError(err error) *commandError {
	return &commandError{Code: 3, Kind: "configuration", Message: err.Error()}
}

func commandName(args []string) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, 3)
	for _, value := range args {
		if strings.HasPrefix(value, "-") {
			break
		}
		parts = append(parts, value)
		if len(parts) == 3 {
			break
		}
	}
	return strings.Join(parts, ".")
}

func environmentLabel(global globalOptions, options Options) string {
	if global.environment != "" {
		return global.environment
	}
	if value, ok := options.LookupEnv("INWX_ENVIRONMENT"); ok && value != "" {
		return value
	}
	return "production"
}

func writeRootHelp(writer io.Writer) {
	_, _ = io.WriteString(writer, `inwx - unofficial community CLI for INWX DNS

Usage:
  inwx [global flags] version
  inwx [global flags] auth check
  inwx [global flags] dns zones list
  inwx [global flags] dns records list <zone> [--type TYPE] [--name NAME]

Global flags:
  --json                         emit versioned JSON
  --environment production|ote  select the fixed INWX endpoint
  --timeout 20s                  command deadline (1s..5m)
  --retries 2                    read retry count (0..5)

This is an unofficial community project. It is not affiliated with, endorsed
by, maintained by, or supported by INWX GmbH. Official services and support:
https://www.inwx.com/
`)
}
