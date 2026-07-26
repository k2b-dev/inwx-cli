package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/k2b-dev/inwx-cli/internal/config"
	"github.com/k2b-dev/inwx-cli/internal/dns"
	"github.com/k2b-dev/inwx-cli/internal/inwx"
	"github.com/k2b-dev/inwx-cli/internal/mutation"
)

type optionalString struct {
	value string
	set   bool
}

func (value *optionalString) String() string { return value.value }
func (value *optionalString) Set(input string) error {
	value.value, value.set = input, true
	return nil
}

type optionalInt struct {
	value int
	set   bool
}

func (value *optionalInt) String() string { return strconv.Itoa(value.value) }
func (value *optionalInt) Set(input string) error {
	parsed, err := strconv.Atoi(input)
	if err != nil {
		return err
	}
	value.value, value.set = parsed, true
	return nil
}

type mutationFlags struct {
	id         string
	recordType string
	name       optionalString
	value      optionalString
	valueFile  string
	valueStdin bool
	ttl        optionalInt
	priority   optionalInt
	expect     string
	apply      bool
}

type mutationOutput struct {
	Operation     string      `json:"operation"`
	Zone          string      `json:"zone"`
	Before        *dns.Record `json:"before"`
	After         *dns.Record `json:"after"`
	Expect        string      `json:"expect"`
	Applied       bool        `json:"applied"`
	Verified      bool        `json:"verified"`
	Recovered     bool        `json:"recovered"`
	Final         *dns.Record `json:"final"`
	DeletedID     string      `json:"deleted_id,omitempty"`
	Authoritative struct {
		FQDN string `json:"fqdn"`
		Type string `json:"type"`
	} `json:"authoritative_verification"`
}

func runRecordMutation(
	ctx context.Context,
	operation string,
	args []string,
	stdout io.Writer,
	global globalOptions,
	environment config.Environment,
	options Options,
) *commandError {
	if len(args) == 1 && isHelp(args[0]) || len(args) == 2 && isHelp(args[1]) {
		writeMutationHelp(stdout, operation)
		return nil
	}
	if len(args) == 0 {
		return usageError(fmt.Errorf("usage: inwx dns records %s <zone>", operation))
	}
	zone, err := dns.NormalizeZone(args[0])
	if err != nil {
		return usageError(err)
	}
	flags, err := parseMutationFlags(operation, args[1:])
	if err != nil {
		return usageError(err)
	}
	if flags.apply && flags.expect == "" {
		return usageError(errors.New("--apply requires --expect from a fresh preview"))
	}
	if !flags.apply && flags.expect != "" {
		return usageError(errors.New("--expect is only valid with --apply"))
	}

	value, hasValue, err := readMutationValue(flags, options.Stdin)
	if err != nil {
		return usageError(err)
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

	plan, requested, planErr := buildMutationPlan(
		operation, environment.Name, zone, records, flags, value, hasValue,
	)
	if planErr != nil {
		return classifyMutation(planErr)
	}
	output := mutationOutput{
		Operation: plan.Operation,
		Zone:      plan.Zone,
		Before:    plan.Before,
		After:     plan.After,
		Expect:    plan.Expect,
	}
	if plan.After != nil {
		output.Authoritative.FQDN = plan.After.FQDN
		output.Authoritative.Type = plan.After.Type
	} else if plan.Before != nil {
		output.Authoritative.FQDN = plan.Before.FQDN
		output.Authoritative.Type = plan.Before.Type
	}
	if !flags.apply {
		return writeMutationOutput(stdout, global.json, environment.Name, output)
	}
	if !mutation.ExpectMatches(flags.expect, plan.Expect) {
		return conflictError(errors.New("state changed since preview; run a fresh preview"))
	}
	if plan.Noop {
		output.Verified = plan.Noop
		output.Final = plan.Before
		return writeMutationOutput(stdout, global.json, environment.Name, output)
	}

	mutationErr, createdID := submitMutation(ctx, client, operation, requested)
	verifiedRecords, readErr := client.ListRecords(ctx, zone)
	verified, final := verifyMutation(operation, verifiedRecords, createdID, requested)
	if readErr == nil && verified {
		output.Applied = !plan.Noop
		output.Verified = true
		output.Recovered = mutationErr != nil
		output.Final = final
		if operation == "delete" {
			output.DeletedID = requested.ID
		}
		return writeMutationOutput(stdout, global.json, environment.Name, output)
	}
	if mutationErr != nil {
		classified := classify(mutationErr, credentials)
		if requested.Value != "" {
			classified.Message = strings.ReplaceAll(classified.Message, requested.Value, "[REDACTED]")
		}
		return classified
	}
	if readErr != nil {
		return verificationError(fmt.Errorf("mutation succeeded but re-read failed: %w", readErr))
	}
	return verificationError(errors.New("mutation succeeded but the requested state was not observed"))
}

func parseMutationFlags(operation string, args []string) (mutationFlags, error) {
	var result mutationFlags
	flags := flag.NewFlagSet("records "+operation, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&result.id, "id", "", "")
	flags.StringVar(&result.recordType, "type", "", "")
	flags.Var(&result.name, "name", "")
	flags.Var(&result.value, "value", "")
	flags.StringVar(&result.valueFile, "value-file", "", "")
	flags.BoolVar(&result.valueStdin, "value-stdin", false, "")
	flags.Var(&result.ttl, "ttl", "")
	flags.Var(&result.priority, "priority", "")
	flags.StringVar(&result.expect, "expect", "", "")
	flags.BoolVar(&result.apply, "apply", false, "")
	if err := flags.Parse(args); err != nil {
		return result, err
	}
	if flags.NArg() != 0 {
		return result, errors.New("unexpected positional arguments")
	}
	switch operation {
	case "create":
		if result.id != "" {
			return result, errors.New("--id is not valid for create")
		}
		if result.recordType == "" || !result.name.set {
			return result, errors.New("create requires --type and --name")
		}
	case "update":
		if result.id == "" {
			return result, errors.New("update requires --id")
		}
		if result.recordType != "" {
			return result, errors.New("--type is not valid for update")
		}
		if !result.name.set && !result.value.set && result.valueFile == "" &&
			!result.valueStdin && !result.ttl.set && !result.priority.set {
			return result, errors.New("update requires at least one changed field")
		}
	case "delete":
		if result.id == "" {
			return result, errors.New("delete requires --id")
		}
		if result.recordType != "" || result.name.set || result.value.set ||
			result.valueFile != "" || result.valueStdin || result.ttl.set || result.priority.set {
			return result, errors.New("delete only accepts --id, --expect, and --apply")
		}
	}
	return result, nil
}

func readMutationValue(flags mutationFlags, stdin io.Reader) (string, bool, error) {
	sources := 0
	if flags.value.set {
		sources++
	}
	if flags.valueFile != "" {
		sources++
	}
	if flags.valueStdin {
		sources++
	}
	if sources > 1 {
		return "", false, errors.New("--value, --value-file, and --value-stdin are mutually exclusive")
	}
	if flags.value.set {
		return flags.value.value, true, nil
	}
	var reader io.Reader
	if flags.valueFile != "" {
		file, err := os.Open(flags.valueFile)
		if err != nil {
			return "", false, fmt.Errorf("open --value-file: %w", err)
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return "", false, fmt.Errorf("inspect --value-file: %w", err)
		}
		if !info.Mode().IsRegular() {
			return "", false, errors.New("--value-file must be a regular file")
		}
		reader = file
	}
	if flags.valueStdin {
		if stdin == nil {
			stdin = os.Stdin
		}
		reader = stdin
	}
	if reader == nil {
		return "", false, nil
	}
	content, err := io.ReadAll(io.LimitReader(reader, 64<<10+1))
	if err != nil {
		return "", false, fmt.Errorf("read record value: %w", err)
	}
	if len(content) > 64<<10 {
		return "", false, errors.New("record value exceeds 64 KiB")
	}
	value := strings.TrimSuffix(string(content), "\n")
	value = strings.TrimSuffix(value, "\r")
	return value, true, nil
}

func buildMutationPlan(
	operation, environment, zone string,
	records []dns.Record,
	flags mutationFlags,
	value string,
	hasValue bool,
) (mutation.Plan, dns.Record, error) {
	switch operation {
	case "create":
		if !hasValue {
			return mutation.Plan{}, dns.Record{}, errors.New("create requires exactly one value source")
		}
		ttl := 3600
		if flags.ttl.set {
			ttl = flags.ttl.value
		}
		priority, err := priorityPointer(flags.priority)
		if err != nil {
			return mutation.Plan{}, dns.Record{}, err
		}
		requested, err := dns.NewRecord(zone, flags.name.value, flags.recordType, value, ttl, priority)
		if err != nil {
			return mutation.Plan{}, dns.Record{}, err
		}
		plan, err := mutation.Create(environment, records, requested)
		return plan, requested, err
	case "update":
		current, err := exactCLIRecord(records, flags.id)
		if err != nil {
			return mutation.Plan{}, dns.Record{}, err
		}
		name := current.Name
		if flags.name.set {
			name = flags.name.value
		}
		if !hasValue {
			value = current.Value
		}
		ttl := current.TTL
		if flags.ttl.set {
			ttl = flags.ttl.value
		}
		priority := current.Priority
		if flags.priority.set {
			priority, err = priorityPointer(flags.priority)
			if err != nil {
				return mutation.Plan{}, dns.Record{}, err
			}
		}
		requested, err := dns.NewRecord(zone, name, current.Type, value, ttl, priority)
		if err != nil {
			return mutation.Plan{}, dns.Record{}, err
		}
		requested.ID = current.ID
		plan, err := mutation.Update(environment, records, flags.id, requested)
		return plan, requested, err
	case "delete":
		plan, err := mutation.Delete(environment, records, zone, flags.id)
		if err != nil {
			return mutation.Plan{}, dns.Record{}, err
		}
		return plan, *plan.Before, nil
	default:
		return mutation.Plan{}, dns.Record{}, errors.New("unknown mutation operation")
	}
}

func exactCLIRecord(records []dns.Record, id string) (dns.Record, error) {
	var result dns.Record
	count := 0
	for _, record := range records {
		if record.ID == id {
			result, count = record, count+1
		}
	}
	if count == 0 {
		return dns.Record{}, &mutation.ConflictError{Message: "record ID does not exist"}
	}
	if count > 1 {
		return dns.Record{}, &mutation.ConflictError{Message: "record ID is ambiguous"}
	}
	return result, nil
}

func priorityPointer(value optionalInt) (*uint16, error) {
	if !value.set {
		return nil, nil
	}
	if value.value < 0 || value.value > 65535 {
		return nil, errors.New("priority must be between 0 and 65535")
	}
	priority := uint16(value.value)
	return &priority, nil
}

func submitMutation(
	ctx context.Context,
	client *inwx.Client,
	operation string,
	requested dns.Record,
) (error, string) {
	switch operation {
	case "create":
		id, err := client.CreateRecord(ctx, requested)
		return err, id
	case "update":
		return client.UpdateRecord(ctx, requested), requested.ID
	case "delete":
		return client.DeleteRecord(ctx, requested.ID), requested.ID
	default:
		return errors.New("unknown mutation operation"), ""
	}
}

func verifyMutation(
	operation string,
	records []dns.Record,
	id string,
	requested dns.Record,
) (bool, *dns.Record) {
	switch operation {
	case "create":
		if id != "" {
			final, ok := mutation.VerifyCreate(records, id, requested)
			return ok, final
		}
		final, ok := mutation.RecoverCreate(records, requested)
		return ok, final
	case "update":
		final, ok := mutation.VerifyUpdate(records, requested)
		return ok, final
	case "delete":
		return mutation.VerifyDelete(records, requested.ID), nil
	default:
		return false, nil
	}
}

func classifyMutation(err error) *commandError {
	var conflict *mutation.ConflictError
	if errors.As(err, &conflict) {
		return conflictError(err)
	}
	return usageError(err)
}

func conflictError(err error) *commandError {
	return &commandError{Code: 5, Kind: "conflict", Message: err.Error()}
}

func verificationError(err error) *commandError {
	return &commandError{Code: 6, Kind: "verification", Message: err.Error()}
}

func writeMutationOutput(
	writer io.Writer,
	jsonMode bool,
	environment string,
	output mutationOutput,
) *commandError {
	if jsonMode {
		return writeSuccess(writer, "dns.records."+output.Operation, environment, output)
	}
	before, _ := json.Marshal(output.Before)
	after, _ := json.Marshal(output.After)
	_, _ = fmt.Fprintf(writer,
		"Operation: %s\nZone: %s\nBefore: %s\nAfter: %s\nExpect: %s\nApplied: %t\nVerified: %t\n",
		output.Operation, output.Zone, before, after, output.Expect, output.Applied, output.Verified,
	)
	if output.Authoritative.FQDN != "" {
		_, _ = fmt.Fprintf(
			writer,
			"Authoritative verification: query %s %s in %s after DNS propagation\n",
			output.Authoritative.Type,
			output.Authoritative.FQDN,
			environment,
		)
	}
	return nil
}

func writeMutationHelp(writer io.Writer, operation string) {
	switch operation {
	case "create":
		_, _ = fmt.Fprintln(writer, "Usage: inwx [global flags] dns records create <zone> --type TYPE --name NAME (--value VALUE | --value-file PATH | --value-stdin) [--ttl SECONDS] [--priority NUMBER] [--expect TOKEN --apply]")
	case "update":
		_, _ = fmt.Fprintln(writer, "Usage: inwx [global flags] dns records update <zone> --id ID [--name NAME] [--value VALUE | --value-file PATH | --value-stdin] [--ttl SECONDS] [--priority NUMBER] [--expect TOKEN --apply]")
	case "delete":
		_, _ = fmt.Fprintln(writer, "Usage: inwx [global flags] dns records delete <zone> --id ID [--expect TOKEN --apply]")
	}
}
