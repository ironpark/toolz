package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestGenerateSelfSignedCertificate(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	got, err := generateSelfSignedCertificate(SelfSignedCertificateRequest{
		Name: "openssl-local", CommonName: "truenas.local", Country: "kr", State: "Seoul", Locality: "Seoul", Organization: "MyHomeNAS", OrganizationalUnit: "IT",
		IPAddresses: []string{" 192.168.1.100 ", ""}, DNSNames: []string{"truenas.local", "*.truenas.local", "TRUENAS.LOCAL"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(got.Certificate))
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("certificate PEM = %q", got.Certificate)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if cert.Subject.CommonName != "truenas.local" || cert.Subject.Country[0] != "KR" || cert.Subject.Organization[0] != "MyHomeNAS" {
		t.Errorf("subject = %v", cert.Subject)
	}
	if len(cert.IPAddresses) != 1 || cert.IPAddresses[0].String() != "192.168.1.100" {
		t.Errorf("ip addresses = %v", cert.IPAddresses)
	}
	if strings.Join(cert.DNSNames, ",") != "truenas.local,*.truenas.local" {
		t.Errorf("dns names = %v", cert.DNSNames)
	}
	if days := cert.NotAfter.Sub(now).Hours() / 24; days != maxSelfSignedDays {
		t.Errorf("validity days = %v", days)
	}
	if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 || cert.KeyUsage&x509.KeyUsageKeyEncipherment == 0 || cert.KeyUsage&x509.KeyUsageContentCommitment == 0 {
		t.Errorf("key usage = %v", cert.KeyUsage)
	}
	if len(cert.ExtKeyUsage) != 1 || cert.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Errorf("ext key usage = %v", cert.ExtKeyUsage)
	}
	if err := cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature); err != nil {
		t.Errorf("self signature: %v", err)
	}
	keyBlock, _ := pem.Decode([]byte(got.PrivateKey))
	if keyBlock == nil || keyBlock.Type != "PRIVATE KEY" {
		t.Fatalf("private key PEM = %q", got.PrivateKey)
	}
	if _, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CN = truenas.local", "IP.1   = 192.168.1.100", "DNS.2  = *.truenas.local", "keyUsage = nonRepudiation, digitalSignature, keyEncipherment", "-days 825"} {
		if !strings.Contains(got.OpenSSLConfig, want) {
			t.Errorf("openssl config missing %q:\n%s", want, got.OpenSSLConfig)
		}
	}

	dir := t.TempDir()
	if out, err := writeCertificateFiles(filepath.Join(dir, "nas.crt"), got); err != nil || out != dir {
		t.Fatalf("writeCertificateFiles() = %q, %v", out, err)
	}
	for _, name := range []string{"nas.crt", "nas.key", "san.cnf"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Error(err)
		}
	}
}

func TestGenerateSelfSignedCertificateValidation(t *testing.T) {
	for name, input := range map[string]SelfSignedCertificateRequest{
		"no name":    {CommonName: "a", DNSNames: []string{"a"}},
		"no san":     {Name: "x", CommonName: "a"},
		"too long":   {Name: "x", CommonName: "a", DNSNames: []string{"a"}, Days: 826},
		"bad ip":     {Name: "x", CommonName: "a", IPAddresses: []string{"300.1.1.1"}},
		"bad c":      {Name: "x", CommonName: "a", DNSNames: []string{"a"}, Country: "KOR"},
		"bad keybit": {Name: "x", CommonName: "a", DNSNames: []string{"a"}, KeyBits: 1024},
	} {
		if _, err := generateSelfSignedCertificate(input, time.Now()); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestSelfSignedCertificateDefaults(t *testing.T) {
	service := &TrueNASService{endpoint: "wss://192.168.1.100/api/current", system: SystemInfo{Hostname: "vault"}}
	got := service.SelfSignedCertificateDefaults()
	if got.CommonName != "vault.local" || strings.Join(got.IPAddresses, ",") != "192.168.1.100" || strings.Join(got.DNSNames, ",") != "vault.local,*.vault.local" || got.Days != 825 {
		t.Errorf("defaults = %#v", got)
	}
}

func TestCertificateManagement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		auth := readServiceRequest(t, conn)
		writeServiceResponse(t, conn, auth.ID, map[string]any{"response_type": "SUCCESS"})
		info := readServiceRequest(t, conn)
		writeServiceResponse(t, conn, info.ID, map[string]any{"hostname": "vault"})

		for _, expected := range []string{"certificate.query", "system.general.config", "certificate.create", "core.job_wait", "system.general.update", "system.general.ui_restart", "system.general.update", "system.general.ui_restart", "certificate.delete", "core.job_wait"} {
			req := readServiceRequest(t, conn)
			if req.Method != expected {
				t.Errorf("method = %q, want %q", req.Method, expected)
			}
			var result any
			switch expected {
			case "certificate.query":
				result = []any{
					map[string]any{"id": 1, "name": "truenas_default", "common": "localhost", "san": []string{"DNS:localhost"}, "issuer": "self-signed", "from": "Mon", "until": "Tue", "cert_type": "CERTIFICATE", "expired": false, "privatekey": "-----BEGIN PRIVATE KEY-----"},
					map[string]any{"id": 7, "name": "openssl-local", "common": "truenas.local", "san": []string{"IP:192.168.1.100"}, "issuer": map[string]any{"name": "ca"}, "from": "Mon", "until": "Tue", "cert_type": "CERTIFICATE", "expired": false},
				}
			case "system.general.config":
				result = map[string]any{"ui_certificate": map[string]any{"id": 7}}
			case "certificate.create":
				data := req.Params[0].(map[string]any)
				if data["create_type"] != "CERTIFICATE_CREATE_IMPORTED" || data["name"] != "openssl-local" || !strings.HasPrefix(data["certificate"].(string), "-----BEGIN CERTIFICATE-----") {
					t.Errorf("certificate.create params = %#v", req.Params)
				}
				result = 42
			case "core.job_wait":
				result = map[string]any{"id": 8, "name": "openssl-local"}
			case "system.general.update":
				if req.Params[0].(map[string]any)["ui_certificate"] != float64(8) && req.Params[0].(map[string]any)["ui_certificate"] != float64(1) {
					t.Errorf("system.general.update params = %#v", req.Params)
				}
			case "system.general.ui_restart":
				if req.Params[0] != float64(3) {
					t.Errorf("ui_restart params = %#v", req.Params)
				}
			case "certificate.delete":
				if req.Params[0] != float64(7) {
					t.Errorf("certificate.delete params = %#v", req.Params)
				}
				result = 43
			}
			writeServiceResponse(t, conn, req.ID, result)
		}
		_, _, _ = conn.Read(context.Background())
	}))
	defer server.Close()

	service := &TrueNASService{profilesPath: t.TempDir() + "/servers.json"}
	if _, err := service.Connect(strings.Replace(server.URL, "http://", "ws://", 1), "admin", "password", "password", false, false, false); err != nil {
		t.Fatal(err)
	}
	overview, err := service.CertificateOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.UICertificateID != 7 || len(overview.Certificates) != 2 || !overview.Certificates[1].UIActive || overview.Certificates[0].UIActive || !overview.Certificates[0].HasPrivateKey || overview.Certificates[1].Issuer != "ca" {
		t.Fatalf("CertificateOverview() = %#v", overview)
	}
	id, err := service.InstallCertificate(CertificateInstall{Name: "openssl-local", Certificate: "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----", PrivateKey: "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----", ApplyToUI: true})
	if err != nil || id != 8 {
		t.Fatalf("InstallCertificate() = %d, %v", id, err)
	}
	if err := service.SetUICertificate(1); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteCertificate(7); err != nil {
		t.Fatal(err)
	}
	if err := service.Disconnect(); err != nil {
		t.Fatal(err)
	}
}
