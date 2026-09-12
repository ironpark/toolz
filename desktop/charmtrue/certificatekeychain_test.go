package main

import (
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func TestCertificateForKeychain(t *testing.T) {
	cert, err := generateSelfSignedCertificate(SelfSignedCertificateRequest{Name: "keychain-test", CommonName: "nas.local", DNSNames: []string{"nas.local"}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	public, fingerprint, err := certificateForKeychain(cert.Certificate + cert.PrivateKey + cert.Certificate)
	if err != nil {
		t.Fatal(err)
	}
	block, rest := pem.Decode(public)
	if block == nil || block.Type != "CERTIFICATE" || len(rest) != 0 || strings.Contains(string(public), "PRIVATE KEY") {
		t.Fatal("import must contain exactly one public certificate")
	}
	if len(fingerprint) != 64 {
		t.Fatalf("invalid fingerprint: %q", fingerprint)
	}
	for _, input := range []string{"", "not a certificate", cert.PrivateKey, "-----BEGIN CERTIFICATE-----\nYWJj\n-----END CERTIFICATE-----"} {
		if _, _, err := certificateForKeychain(input); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
}
