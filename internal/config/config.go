package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const maxCredentialSize = 64 << 10

type LookupEnv func(string) (string, bool)

type Credentials struct {
	Username     string
	Password     string
	SharedSecret string
}

type Environment struct {
	Name     string
	Endpoint string
}

func ResolveEnvironment(flagValue string, lookup LookupEnv) (Environment, error) {
	value := flagValue
	if value == "" {
		if configured, ok := lookup("INWX_ENVIRONMENT"); ok {
			value = configured
		}
	}
	if value == "" {
		value = "production"
	}

	switch value {
	case "production":
		return Environment{
			Name:     value,
			Endpoint: "https://api.domrobot.com/jsonrpc/",
		}, nil
	case "ote":
		return Environment{
			Name:     value,
			Endpoint: "https://api.ote.domrobot.com/jsonrpc/",
		}, nil
	default:
		return Environment{}, fmt.Errorf(
			"invalid environment %q: expected production or ote",
			value,
		)
	}
}

func LoadCredentials(lookup LookupEnv) (Credentials, error) {
	username, err := readCredential(lookup, "INWX_USERNAME")
	if err != nil {
		return Credentials{}, err
	}
	password, err := readCredential(lookup, "INWX_PASSWORD")
	if err != nil {
		return Credentials{}, err
	}
	sharedSecret, err := readCredential(lookup, "INWX_SHARED_SECRET")
	if err != nil {
		return Credentials{}, err
	}

	if username == "" {
		return Credentials{}, errors.New("INWX_USERNAME or INWX_USERNAME_FILE is required")
	}
	if password == "" {
		return Credentials{}, errors.New("INWX_PASSWORD or INWX_PASSWORD_FILE is required")
	}

	return Credentials{
		Username:     username,
		Password:     password,
		SharedSecret: sharedSecret,
	}, nil
}

func readCredential(lookup LookupEnv, name string) (string, error) {
	direct, directSet := lookup(name)
	path, fileSet := lookup(name + "_FILE")

	if directSet && fileSet {
		return "", fmt.Errorf("%s and %s_FILE cannot both be set", name, name)
	}
	if directSet {
		return direct, nil
	}
	if !fileSet {
		return "", nil
	}
	if path == "" {
		return "", fmt.Errorf("%s_FILE is empty", name)
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read %s_FILE: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s_FILE must reference a regular file", name)
	}
	if info.Size() > maxCredentialSize {
		return "", fmt.Errorf("%s_FILE exceeds 64 KiB", name)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s_FILE: %w", name, err)
	}
	if len(content) > maxCredentialSize {
		return "", fmt.Errorf("%s_FILE exceeds 64 KiB", name)
	}

	value := string(content)
	if strings.HasSuffix(value, "\r\n") {
		value = strings.TrimSuffix(value, "\r\n")
	} else {
		value = strings.TrimSuffix(value, "\n")
	}
	return value, nil
}
