package main

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func (s *TrueNASService) CertificateKeychainSupported() bool {
	return runtime.GOOS == "darwin"
}

// ImportCertificateToKeychain imports the public certificate, without its key
// or an automatic trust override, into the current user's default keychain.
func (s *TrueNASService) ImportCertificateToKeychain(id int) (string, error) {
	if !s.CertificateKeychainSupported() {
		return "", errors.New("인증서 키체인 등록은 macOS에서 지원합니다")
	}
	if id <= 0 {
		return "", errors.New("인증서 ID가 필요합니다")
	}
	client, err := s.connectedClient()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	var certificates []struct {
		ID          int    `json:"id"`
		Certificate string `json:"certificate"`
	}
	if err := client.Call(ctx, "certificate.query", []any{[][]any{{"id", "=", id}}, map[string]any{"select": []string{"id", "certificate"}}}, &certificates); err != nil {
		return "", fmt.Errorf("공개 인증서 조회 실패: %w", err)
	}
	if len(certificates) != 1 || certificates[0].ID != id {
		return "", errors.New("선택한 인증서를 찾을 수 없습니다")
	}
	publicPEM, fingerprint, err := certificateForKeychain(certificates[0].Certificate)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "charmtrue-public-certificate-*.pem")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(publicPEM); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	output, err := exec.CommandContext(ctx, "/usr/bin/security", "add-certificates", file.Name()).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("키체인 인증서 등록 실패: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return fingerprint, nil
}

func certificateForKeychain(input string) ([]byte, string, error) {
	block, _ := pem.Decode([]byte(input))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, "", errors.New("등록할 공개 PEM 인증서가 없습니다")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, "", fmt.Errorf("인증서 형식이 올바르지 않습니다: %w", err)
	}
	// Re-encode only the selected leaf certificate; never import accompanying
	// private keys or silently promote an accompanying CA to a trust anchor.
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), fmt.Sprintf("%X", sha256.Sum256(cert.Raw)), nil
}
