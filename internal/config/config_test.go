package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveEnvironment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		flag     string
		values   map[string]string
		expected string
	}{
		{name: "default", expected: "production"},
		{name: "environment", values: map[string]string{"INWX_ENVIRONMENT": "ote"}, expected: "ote"},
		{name: "flag wins", flag: "production", values: map[string]string{"INWX_ENVIRONMENT": "ote"}, expected: "production"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			environment, err := ResolveEnvironment(test.flag, mapLookup(test.values))
			if err != nil {
				t.Fatal(err)
			}
			if environment.Name != test.expected {
				t.Fatalf("got %q, want %q", environment.Name, test.expected)
			}
		})
	}
}

func TestResolveEnvironmentRejectsUnknownValue(t *testing.T) {
	t.Parallel()

	_, err := ResolveEnvironment("staging", mapLookup(nil))
	if err == nil {
		t.Fatal("expected invalid environment error")
	}
}

func TestLoadCredentialsFromFiles(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	usernamePath := filepath.Join(directory, "username")
	passwordPath := filepath.Join(directory, "password")
	if err := os.WriteFile(usernamePath, []byte("operator\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(passwordPath, []byte(" leading and trailing \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	credentials, err := LoadCredentials(mapLookup(map[string]string{
		"INWX_USERNAME_FILE": usernamePath,
		"INWX_PASSWORD_FILE": passwordPath,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Username != "operator" {
		t.Fatalf("unexpected username %q", credentials.Username)
	}
	if credentials.Password != " leading and trailing " {
		t.Fatalf("credential whitespace changed: %q", credentials.Password)
	}
}

func TestLoadCredentialsRejectsConflictingSources(t *testing.T) {
	t.Parallel()

	_, err := LoadCredentials(mapLookup(map[string]string{
		"INWX_USERNAME":      "operator",
		"INWX_USERNAME_FILE": "/unused",
		"INWX_PASSWORD":      "value",
	}))
	if err == nil || !strings.Contains(err.Error(), "cannot both be set") {
		t.Fatalf("expected source conflict, got %v", err)
	}
}

func TestLoadCredentialsRejectsNonRegularFile(t *testing.T) {
	t.Parallel()

	_, err := LoadCredentials(mapLookup(map[string]string{
		"INWX_USERNAME_FILE": t.TempDir(),
		"INWX_PASSWORD":      "value",
	}))
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("expected regular file error, got %v", err)
	}
}

func mapLookup(values map[string]string) LookupEnv {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}
