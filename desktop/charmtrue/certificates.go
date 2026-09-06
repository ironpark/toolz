package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// maxSelfSignedDays is the longest validity Chrome and Safari accept for a
// self-signed server certificate before refusing the connection.
const maxSelfSignedDays = 825

// CertificateInfo is a frontend-friendly summary of a TrueNAS certificate.
type CertificateInfo struct {
	ID              int      `json:"id"`
	Name            string   `json:"name"`
	CommonName      string   `json:"commonName"`
	SubjectAltNames []string `json:"subjectAltNames"`
	Issuer          string   `json:"issuer"`
	From            string   `json:"from"`
	Until           string   `json:"until"`
	Type            string   `json:"type"`
	Expired         bool     `json:"expired"`
	HasPrivateKey   bool     `json:"hasPrivateKey"`
	UIActive        bool     `json:"uiActive"`
}

// CertificateOverview lists certificates plus the one bound to the web UI.
type CertificateOverview struct {
	Certificates    []CertificateInfo `json:"certificates"`
	UICertificateID int               `json:"uiCertificateId"`
}

// SelfSignedCertificateRequest mirrors the fields of an OpenSSL san.cnf.
type SelfSignedCertificateRequest struct {
	Name               string   `json:"name"`
	CommonName         string   `json:"commonName"`
	Country            string   `json:"country"`
	State              string   `json:"state"`
	Locality           string   `json:"locality"`
	Organization       string   `json:"organization"`
	OrganizationalUnit string   `json:"organizationalUnit"`
	IPAddresses        []string `json:"ipAddresses"`
	DNSNames           []string `json:"dnsNames"`
	Days               int      `json:"days"`
	KeyBits            int      `json:"keyBits"`
}

// GeneratedCertificate holds PEM output equivalent to `openssl req -x509`.
type GeneratedCertificate struct {
	Name          string `json:"name"`
	Certificate   string `json:"certificate"`
	PrivateKey    string `json:"privateKey"`
	OpenSSLConfig string `json:"opensslConfig"`
	NotAfter      string `json:"notAfter"`
	Fingerprint   string `json:"fingerprint"`
}

// CertificateInstall imports a certificate and optionally binds it to the UI.
type CertificateInstall struct {
	Name        string `json:"name"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"privateKey"`
	ApplyToUI   bool   `json:"applyToUi"`
}

type apiCertificate struct {
	ID         int             `json:"id"`
	Name       string          `json:"name"`
	Common     string          `json:"common"`
	SAN        []string        `json:"san"`
	Issuer     json.RawMessage `json:"issuer"`
	From       string          `json:"from"`
	Until      string          `json:"until"`
	CertType   string          `json:"cert_type"`
	Expired    bool            `json:"expired"`
	PrivateKey string          `json:"privatekey"`
}

// SelfSignedCertificateDefaults pre-fills the request from the current
// connection so the user only has to confirm the hosts they reach TrueNAS by.
func (s *TrueNASService) SelfSignedCertificateDefaults() SelfSignedCertificateRequest {
	s.mu.RLock()
	endpoint, hostname := s.endpoint, s.system.Hostname
	s.mu.RUnlock()

	req := SelfSignedCertificateRequest{Name: "charmtrue-local", Country: "KR", State: "Seoul", Locality: "Seoul", Organization: "MyHomeNAS", OrganizationalUnit: "IT", Days: maxSelfSignedDays, KeyBits: 2048}
	host := ""
	if u, err := url.Parse(endpoint); err == nil {
		host = u.Hostname()
	}
	if ip := net.ParseIP(host); ip != nil {
		req.IPAddresses = append(req.IPAddresses, host)
	} else if host != "" {
		req.DNSNames = append(req.DNSNames, host)
	}
	hostname = strings.TrimSpace(hostname)
	if hostname != "" && !strings.Contains(hostname, ".") {
		hostname += ".local"
	}
	if hostname != "" && !containsFold(req.DNSNames, hostname) {
		req.DNSNames = append(req.DNSNames, hostname)
	}
	if len(req.DNSNames) > 0 {
		req.CommonName = req.DNSNames[0]
		req.DNSNames = append(req.DNSNames, "*."+req.DNSNames[0])
	} else if len(req.IPAddresses) > 0 {
		req.CommonName = req.IPAddresses[0]
	} else {
		req.CommonName = "truenas.local"
		req.DNSNames = []string{"truenas.local", "*.truenas.local"}
	}
	return req
}

// GenerateSelfSignedCertificate produces a SAN certificate and key without
// requiring OpenSSL on the workstation. It does not need a TrueNAS connection.
func (s *TrueNASService) GenerateSelfSignedCertificate(input SelfSignedCertificateRequest) (GeneratedCertificate, error) {
	return generateSelfSignedCertificate(input, time.Now())
}

func generateSelfSignedCertificate(input SelfSignedCertificateRequest, now time.Time) (GeneratedCertificate, error) {
	input, err := normalizeCertificateRequest(input)
	if err != nil {
		return GeneratedCertificate{}, err
	}
	var ips []net.IP
	for _, raw := range input.IPAddresses {
		ips = append(ips, net.ParseIP(raw))
	}
	key, err := rsa.GenerateKey(rand.Reader, input.KeyBits)
	if err != nil {
		return GeneratedCertificate{}, fmt.Errorf("개인키 생성 실패: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return GeneratedCertificate{}, fmt.Errorf("일련번호 생성 실패: %w", err)
	}
	subject := pkix.Name{CommonName: input.CommonName}
	if input.Country != "" {
		subject.Country = []string{input.Country}
	}
	if input.State != "" {
		subject.Province = []string{input.State}
	}
	if input.Locality != "" {
		subject.Locality = []string{input.Locality}
	}
	if input.Organization != "" {
		subject.Organization = []string{input.Organization}
	}
	if input.OrganizationalUnit != "" {
		subject.OrganizationalUnit = []string{input.OrganizationalUnit}
	}
	notBefore := now.Add(-5 * time.Minute).UTC()
	notAfter := now.Add(time.Duration(input.Days) * 24 * time.Hour).UTC()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               subject,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageContentCommitment | x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	template.DNSNames = input.DNSNames
	template.IPAddresses = ips
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return GeneratedCertificate{}, fmt.Errorf("인증서 생성 실패: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return GeneratedCertificate{}, fmt.Errorf("개인키 인코딩 실패: %w", err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		return GeneratedCertificate{}, fmt.Errorf("인증서 검증 실패: %w", err)
	}
	return GeneratedCertificate{
		Name:          input.Name,
		Certificate:   string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		PrivateKey:    string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})),
		OpenSSLConfig: opensslConfig(input),
		NotAfter:      parsed.NotAfter.Format(time.RFC3339),
		Fingerprint:   certificateFingerprint(der),
	}, nil
}

func normalizeCertificateRequest(input SelfSignedCertificateRequest) (SelfSignedCertificateRequest, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.CommonName = strings.TrimSpace(input.CommonName)
	input.Country = strings.ToUpper(strings.TrimSpace(input.Country))
	input.State = strings.TrimSpace(input.State)
	input.Locality = strings.TrimSpace(input.Locality)
	input.Organization = strings.TrimSpace(input.Organization)
	input.OrganizationalUnit = strings.TrimSpace(input.OrganizationalUnit)
	if input.Name == "" {
		return input, errors.New("인증서 이름이 필요합니다")
	}
	if input.CommonName == "" {
		return input, errors.New("CN(Common Name)이 필요합니다")
	}
	if input.Country != "" && len(input.Country) != 2 {
		return input, errors.New("국가 코드는 두 글자여야 합니다 (예: KR)")
	}
	if input.Days <= 0 {
		input.Days = maxSelfSignedDays
	}
	if input.Days > maxSelfSignedDays {
		return input, fmt.Errorf("유효기간은 최대 %d일까지 가능합니다 (브라우저 제한)", maxSelfSignedDays)
	}
	switch input.KeyBits {
	case 0:
		input.KeyBits = 2048
	case 2048, 3072, 4096:
	default:
		return input, errors.New("키 길이는 2048, 3072, 4096 중 하나여야 합니다")
	}
	var ips, names []string
	for _, raw := range input.IPAddresses {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if net.ParseIP(raw) == nil {
			return input, fmt.Errorf("올바르지 않은 IP 주소: %s", raw)
		}
		if !containsFold(ips, raw) {
			ips = append(ips, raw)
		}
	}
	for _, raw := range input.DNSNames {
		raw = strings.ToLower(strings.TrimSpace(raw))
		if raw == "" {
			continue
		}
		if strings.ContainsAny(raw, " /:") {
			return input, fmt.Errorf("올바르지 않은 도메인 이름: %s", raw)
		}
		if !containsFold(names, raw) {
			names = append(names, raw)
		}
	}
	if len(ips) == 0 && len(names) == 0 {
		return input, errors.New("IP 주소 또는 도메인 이름을 하나 이상 입력하세요")
	}
	input.IPAddresses, input.DNSNames = ips, names
	return input, nil
}

func containsFold(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

// opensslConfig renders the equivalent san.cnf so the user can reproduce the
// certificate with OpenSSL later if desired.
func opensslConfig(input SelfSignedCertificateRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[req]\ndefault_bits        = %d\ndistinguished_name  = req_distinguished_name\nreq_extensions      = v3_req\nx509_extensions     = v3_req\nprompt              = no\n\n[req_distinguished_name]\n", input.KeyBits)
	for _, kv := range [][2]string{{"C", input.Country}, {"ST", input.State}, {"L", input.Locality}, {"O", input.Organization}, {"OU", input.OrganizationalUnit}, {"CN", input.CommonName}} {
		if kv[1] != "" {
			fmt.Fprintf(&b, "%-2s = %s\n", kv[0], kv[1])
		}
	}
	b.WriteString("\n[v3_req]\nkeyUsage = nonRepudiation, digitalSignature, keyEncipherment\nextendedKeyUsage = serverAuth\nsubjectAltName = @alt_names\n\n[alt_names]\n")
	for i, ip := range input.IPAddresses {
		fmt.Fprintf(&b, "IP.%d   = %s\n", i+1, ip)
	}
	for i, name := range input.DNSNames {
		fmt.Fprintf(&b, "DNS.%d  = %s\n", i+1, name)
	}
	fmt.Fprintf(&b, "\n# openssl req -x509 -nodes -days %d -newkey rsa:%d -keyout %s.key -out %s.crt -config san.cnf\n", input.Days, input.KeyBits, input.Name, input.Name)
	return b.String()
}

func certificateFingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	parts := make([]string, len(sum))
	for i, x := range sum {
		parts[i] = fmt.Sprintf("%02X", x)
	}
	return strings.Join(parts, ":")
}

// SaveCertificateFiles asks for a destination via the native save dialog and
// writes <name>.crt, <name>.key and san.cnf side by side. It returns the
// directory that received the files, or "" when the user cancelled.
func (s *TrueNASService) SaveCertificateFiles(cert GeneratedCertificate) (string, error) {
	if strings.TrimSpace(cert.Certificate) == "" || strings.TrimSpace(cert.PrivateKey) == "" {
		return "", errors.New("저장할 인증서가 없습니다")
	}
	app := application.Get()
	if app == nil {
		return "", errors.New("파일 저장 대화상자를 사용할 수 없습니다")
	}
	name := sanitizeFileName(cert.Name)
	target, err := app.Dialog.SaveFile().SetFilename(name + ".crt").SetMessage("인증서(.crt)를 저장할 위치를 고르세요. 개인키(.key)와 san.cnf가 같은 폴더에 함께 저장됩니다.").PromptForSingleSelection()
	if err != nil {
		return "", fmt.Errorf("저장 위치 선택 실패: %w", err)
	}
	if target == "" {
		return "", nil
	}
	return writeCertificateFiles(target, cert)
}

func writeCertificateFiles(target string, cert GeneratedCertificate) (string, error) {
	dir := filepath.Dir(target)
	base := strings.TrimSuffix(filepath.Base(target), filepath.Ext(target))
	if base == "" {
		base = sanitizeFileName(cert.Name)
	}
	files := []struct {
		name string
		body string
		mode os.FileMode
	}{
		{base + ".crt", cert.Certificate, 0o644},
		{base + ".key", cert.PrivateKey, 0o600},
	}
	if cert.OpenSSLConfig != "" {
		files = append(files, struct {
			name string
			body string
			mode os.FileMode
		}{"san.cnf", cert.OpenSSLConfig, 0o644})
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f.name), []byte(f.body), f.mode); err != nil {
			return "", fmt.Errorf("%s 저장 실패: %w", f.name, err)
		}
	}
	return dir, nil
}

func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "truenas"
	}
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, name)
}

// CertificateOverview returns every certificate and which one the UI uses.
func (s *TrueNASService) CertificateOverview() (CertificateOverview, error) {
	client, err := s.connectedClient()
	if err != nil {
		return CertificateOverview{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	var raw []apiCertificate
	if err := client.Call(ctx, "certificate.query", nil, &raw); err != nil {
		return CertificateOverview{}, fmt.Errorf("인증서 조회 실패: %w", err)
	}
	var general struct {
		UICertificate json.RawMessage `json:"ui_certificate"`
	}
	if err := client.Call(ctx, "system.general.config", nil, &general); err != nil {
		return CertificateOverview{}, fmt.Errorf("GUI 설정 조회 실패: %w", err)
	}
	result := CertificateOverview{UICertificateID: certificateReferenceID(general.UICertificate), Certificates: []CertificateInfo{}}
	for _, x := range raw {
		result.Certificates = append(result.Certificates, CertificateInfo{
			ID: x.ID, Name: x.Name, CommonName: x.Common, SubjectAltNames: x.SAN, Issuer: issuerLabel(x.Issuer),
			From: x.From, Until: x.Until, Type: x.CertType, Expired: x.Expired, HasPrivateKey: strings.TrimSpace(x.PrivateKey) != "",
			UIActive: x.ID == result.UICertificateID && x.ID != 0,
		})
	}
	return result, nil
}

func certificateReferenceID(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var id int
	if json.Unmarshal(raw, &id) == nil {
		return id
	}
	var ref struct {
		ID int `json:"id"`
	}
	if json.Unmarshal(raw, &ref) == nil {
		return ref.ID
	}
	return 0
}

func issuerLabel(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var ref struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &ref) == nil {
		return ref.Name
	}
	return ""
}

// InstallCertificate imports a PEM certificate and private key into TrueNAS
// (Credentials > Certificates > Import) and, when requested, selects it as the
// GUI SSL certificate and restarts the web service.
func (s *TrueNASService) InstallCertificate(input CertificateInstall) (int, error) {
	client, err := s.connectedClient()
	if err != nil {
		return 0, err
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Certificate = strings.TrimSpace(input.Certificate) + "\n"
	input.PrivateKey = strings.TrimSpace(input.PrivateKey) + "\n"
	if input.Name == "" {
		return 0, errors.New("인증서 이름이 필요합니다")
	}
	if !strings.Contains(input.Certificate, "-----BEGIN CERTIFICATE-----") {
		return 0, errors.New("PEM 형식의 인증서가 필요합니다")
	}
	if !strings.Contains(input.PrivateKey, "PRIVATE KEY-----") {
		return 0, errors.New("PEM 형식의 개인키가 필요합니다")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*connectionTimeout)
	defer cancel()
	var jobID int
	payload := map[string]any{"name": input.Name, "create_type": "CERTIFICATE_CREATE_IMPORTED", "certificate": input.Certificate, "privatekey": input.PrivateKey}
	if err := client.Call(ctx, "certificate.create", []any{payload}, &jobID); err != nil {
		return 0, fmt.Errorf("인증서 가져오기 실패: %w", err)
	}
	// TrueNAS returns either the new certificate ID or the full certificate
	// object depending on the version, so decode both shapes.
	var created json.RawMessage
	if err := client.WaitJob(ctx, jobID, &created); err != nil {
		return 0, fmt.Errorf("인증서 가져오기 실패: %w", err)
	}
	id := certificateReferenceID(created)
	if id == 0 {
		return 0, errors.New("TrueNAS가 생성된 인증서 ID를 반환하지 않았습니다")
	}
	if input.ApplyToUI {
		if err := s.applyUICertificate(ctx, client, id); err != nil {
			return id, err
		}
	}
	return id, nil
}

// SetUICertificate binds an existing certificate to the web GUI.
func (s *TrueNASService) SetUICertificate(id int) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	if id <= 0 {
		return errors.New("인증서 ID가 필요합니다")
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	return s.applyUICertificate(ctx, client, id)
}

type apiCaller interface {
	Call(ctx context.Context, method string, params []any, result any) error
}

func (s *TrueNASService) applyUICertificate(ctx context.Context, client apiCaller, id int) error {
	if err := client.Call(ctx, "system.general.update", []any{map[string]any{"ui_certificate": id}}, nil); err != nil {
		return fmt.Errorf("GUI 인증서 설정 실패: %w", err)
	}
	if err := client.Call(ctx, "system.general.ui_restart", []any{3}, nil); err != nil {
		return fmt.Errorf("웹 서비스 재시작 요청 실패: %w", err)
	}
	return nil
}

// DeleteCertificate removes a certificate that is not bound to the UI.
func (s *TrueNASService) DeleteCertificate(id int) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	if id <= 0 {
		return errors.New("인증서 ID가 필요합니다")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*connectionTimeout)
	defer cancel()
	var jobID int
	if err := client.Call(ctx, "certificate.delete", []any{id, true}, &jobID); err != nil {
		return fmt.Errorf("인증서 삭제 실패: %w", err)
	}
	if err := client.WaitJob(ctx, jobID, nil); err != nil {
		return fmt.Errorf("인증서 삭제 실패: %w", err)
	}
	return nil
}
