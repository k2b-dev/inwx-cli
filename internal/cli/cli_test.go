package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRecordsJSONOutput(t *testing.T) {
	t.Parallel()

	server := readOnlyServer(t)
	defer server.Close()
	var stdout, stderr bytes.Buffer
	exitCode := Run(
		context.Background(),
		[]string{
			"--json",
			"--environment", "ote",
			"dns", "records", "list", "example.test", "--type", "MX",
		},
		&stdout,
		&stderr,
		testOptions(server.URL),
	)
	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr %q", stderr.String())
	}

	var envelope struct {
		SchemaVersion string `json:"schema_version"`
		Command       string `json:"command"`
		Environment   string `json:"environment"`
		OK            bool   `json:"ok"`
		Data          struct {
			Records []struct {
				Type     string `json:"type"`
				Priority int    `json:"priority"`
			} `json:"records"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != schemaVersion ||
		envelope.Command != "dns.records.list" ||
		envelope.Environment != "ote" ||
		!envelope.OK ||
		len(envelope.Data.Records) != 1 ||
		envelope.Data.Records[0].Type != "MX" ||
		envelope.Data.Records[0].Priority != 10 {
		t.Fatalf("unexpected envelope %#v", envelope)
	}
}

func TestZonesHumanOutput(t *testing.T) {
	t.Parallel()

	server := readOnlyServer(t)
	defer server.Close()
	var stdout, stderr bytes.Buffer
	exitCode := Run(
		context.Background(),
		[]string{"--environment", "ote", "dns", "zones", "list"},
		&stdout,
		&stderr,
		testOptions(server.URL),
	)
	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", exitCode, stderr.String())
	}
	if stdout.String() != "ZONE           TYPE\nexample.test.  MASTER\n" {
		t.Fatalf("unexpected table %q", stdout.String())
	}
}

func TestEmptyZonesJSONUsesArray(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var call struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Error(err)
			return
		}
		switch call.Method {
		case "account.login":
			writeCLIEnvelope(t, writer, map[string]string{"tfa": "0"})
		case "account.logout":
			writeCLIEnvelope(t, writer, struct{}{})
		case "nameserver.list":
			writeCLIEnvelope(t, writer, map[string]any{
				"count":   0,
				"domains": []any{},
			})
		default:
			t.Errorf("unexpected method %q", call.Method)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	exitCode := Run(
		context.Background(),
		[]string{"--json", "--environment", "ote", "dns", "zones", "list"},
		&stdout,
		&stderr,
		testOptions(server.URL),
	)
	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"zones":[]`) {
		t.Fatalf("empty zones must be an array: %s", stdout.String())
	}
}

func TestConfigurationErrorUsesStderrOnly(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	exitCode := Run(
		context.Background(),
		[]string{"--json", "auth", "check"},
		&stdout,
		&stderr,
		Options{
			LookupEnv: func(string) (string, bool) { return "", false },
		},
	)
	if exitCode != 3 || stdout.Len() != 0 {
		t.Fatalf("exit=%d stdout=%q", exitCode, stdout.String())
	}
	if !strings.Contains(stderr.String(), `"kind":"configuration"`) {
		t.Fatalf("unexpected stderr %q", stderr.String())
	}
}

func TestHelpAtEveryLevel(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		{"--help"},
		{"version", "--help"},
		{"auth", "--help"},
		{"auth", "check", "--help"},
		{"dns", "--help"},
		{"dns", "zones", "--help"},
		{"dns", "zones", "list", "--help"},
		{"dns", "records", "--help"},
		{"dns", "records", "list", "--help"},
		{"dns", "records", "list", "example.test", "--help"},
		{"dns", "records", "create", "--help"},
		{"dns", "records", "create", "example.test", "--help"},
		{"dns", "records", "update", "--help"},
		{"dns", "records", "delete", "--help"},
	}
	for _, args := range cases {
		args := args
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := Run(context.Background(), args, &stdout, &stderr, Options{})
			if exitCode != 0 || stdout.Len() == 0 || stderr.Len() != 0 {
				t.Fatalf(
					"args=%v exit=%d stdout=%q stderr=%q",
					args,
					exitCode,
					stdout.String(),
					stderr.String(),
				)
			}
		})
	}
}

func TestSubprocessVersionSeparatesOutput(t *testing.T) {
	t.Parallel()

	command := helperCommand(t, "--json", "version")
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), `"ok":true`) {
		t.Fatalf("unexpected stdout %q", output)
	}
}

func TestSubprocessRedactsInvalidCredentialError(t *testing.T) {
	t.Parallel()

	secret := "sensitive-" + strings.ReplaceAll(t.Name(), "/", "-")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"code":    2200,
			"msg":     "rejected " + secret,
			"resData": map[string]any{},
		})
	}))
	defer server.Close()

	command := helperCommand(t, "--json", "--environment", "ote", "auth", "check")
	command.Env = append(command.Env,
		"INWX_TEST_ENDPOINT="+server.URL,
		"INWX_USERNAME=operator",
		"INWX_PASSWORD="+secret,
	)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	var exit *exec.ExitError
	if !strings.Contains(stderr.String(), "[REDACTED]") ||
		strings.Contains(stderr.String(), secret) ||
		stdout.Len() != 0 ||
		!errorsAs(err, &exit) ||
		exit.ExitCode() != 3 {
		t.Fatalf(
			"err=%v stdout=%q stderr=%q",
			err,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestCLIHelper(t *testing.T) {
	if os.Getenv("INWX_TEST_HELPER") != "1" {
		return
	}
	marker := -1
	for index, value := range os.Args {
		if value == "--" {
			marker = index
			break
		}
	}
	if marker == -1 {
		os.Exit(99)
	}
	os.Exit(Run(
		context.Background(),
		os.Args[marker+1:],
		os.Stdout,
		os.Stderr,
		Options{
			Version:          "test",
			Commit:           "test-commit",
			Date:             "test-date",
			EndpointOverride: os.Getenv("INWX_TEST_ENDPOINT"),
		},
	))
}

func helperCommand(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	commandArgs := []string{"-test.run=TestCLIHelper", "--"}
	commandArgs = append(commandArgs, args...)
	command := exec.Command(os.Args[0], commandArgs...)
	command.Env = append(os.Environ(), "INWX_TEST_HELPER=1")
	return command
}

func testOptions(endpoint string) Options {
	values := map[string]string{
		"INWX_USERNAME": "operator",
		"INWX_PASSWORD": "credential",
	}
	return Options{
		EndpointOverride: endpoint,
		LookupEnv: func(name string) (string, bool) {
			value, ok := values[name]
			return value, ok
		},
	}
}

func readOnlyServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var call struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Error(err)
			return
		}
		switch call.Method {
		case "account.login":
			writeCLIEnvelope(t, writer, map[string]string{"tfa": "0"})
		case "account.logout":
			writeCLIEnvelope(t, writer, struct{}{})
		case "nameserver.list":
			writeCLIEnvelope(t, writer, map[string]any{
				"count": 1,
				"domains": []map[string]any{{
					"roId":   70001,
					"domain": "example.test",
					"type":   "MASTER",
				}},
			})
		case "nameserver.info":
			content, err := os.ReadFile("../inwx/testdata/nameserver-info-records.json")
			if err != nil {
				t.Error(err)
				return
			}
			_, _ = writer.Write(content)
		default:
			t.Errorf("unexpected method %q", call.Method)
		}
	}))
}

func writeCLIEnvelope(t *testing.T, writer http.ResponseWriter, data any) {
	t.Helper()
	if err := json.NewEncoder(writer).Encode(map[string]any{
		"code":    1000,
		"msg":     "test response",
		"resData": data,
	}); err != nil {
		t.Error(err)
	}
}

func errorsAs(err error, target any) bool {
	switch value := target.(type) {
	case **exec.ExitError:
		exit, ok := err.(*exec.ExitError)
		if ok {
			*value = exit
		}
		return ok
	default:
		return false
	}
}
