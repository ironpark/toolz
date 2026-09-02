package truenas

import (
	"errors"
	"net/http"
	"testing"
)

func TestNormalizeEndpoint(t *testing.T) {
	tests := map[string]string{
		"nas.local":                       "wss://nas.local/api/current",
		"https://nas.local":               "wss://nas.local/api/current",
		"http://nas.local/":               "ws://nas.local/api/current",
		"wss://nas.local/custom-endpoint": "wss://nas.local/custom-endpoint",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := normalizeEndpoint(input)
			if err != nil {
				t.Fatalf("normalizeEndpoint() error = %v", err)
			}
			if got != want {
				t.Fatalf("normalizeEndpoint() = %q, want %q", got, want)
			}
		})
	}
}

func TestConfigAllowsAPIKeyOverWS(t *testing.T) {
	config, err := (Config{Endpoint: "ws://nas.local/api/current", Username: "admin", APIKey: "secret"}).validate()
	if err != nil {
		t.Fatalf("validate() error = %v", err)
	}
	if config.Endpoint != "ws://nas.local/api/current" {
		t.Fatalf("endpoint = %q", config.Endpoint)
	}
}

func TestConfigAllowsPrivateTLSCertificate(t *testing.T) {
	config, err := (Config{Endpoint: "nas.local", InsecureSkipVerify: true}).validate()
	if err != nil {
		t.Fatalf("validate() error = %v", err)
	}
	transport, ok := config.HTTPClient.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("TLS config = %#v", transport)
	}
}

func TestConfigRejectsMultipleCredentials(t *testing.T) {
	_, err := (Config{Endpoint: "nas.local", Username: "admin", APIKey: "key", Password: "password"}).validate()
	if err == nil {
		t.Fatal("validate() error = nil")
	}
}

func TestConfigRejectsOTPWithoutPassword(t *testing.T) {
	_, err := (Config{Endpoint: "nas.local", OTP: "123456"}).validate()
	if err == nil {
		t.Fatal("validate() error = nil")
	}
}

func TestConfigReadLimit(t *testing.T) {
	config, err := (Config{Endpoint: "nas.local"}).validate()
	if err != nil {
		t.Fatalf("validate() error = %v", err)
	}
	if config.ReadLimit != defaultReadLimit {
		t.Fatalf("ReadLimit = %d, want %d", config.ReadLimit, defaultReadLimit)
	}

	_, err = (Config{Endpoint: "nas.local", ReadLimit: -1}).validate()
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "read limit" {
		t.Fatalf("validate() error = %#v, want read limit ValidationError", err)
	}
}
