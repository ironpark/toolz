package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memoryCredentialStore struct {
	secrets map[string]string
}

func (s *memoryCredentialStore) Set(service, account, secret string) error {
	s.secrets[service+"/"+account] = secret
	return nil
}

func (s *memoryCredentialStore) Get(service, account string) (string, error) {
	secret, ok := s.secrets[service+"/"+account]
	if !ok {
		return "", errors.New("not found")
	}
	return secret, nil
}

func (s *memoryCredentialStore) Delete(service, account string) error {
	delete(s.secrets, service+"/"+account)
	return nil
}

func TestSavedServersUpsertAndDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "servers.json")
	credentials := &memoryCredentialStore{secrets: map[string]string{}}
	service := &TrueNASService{profilesPath: path, credentials: credentials}
	profile := SavedServer{
		Name:                    "vault",
		Endpoint:                "truenas.local",
		Username:                "admin",
		AuthenticationMethod:    "api_key",
		AllowPrivateCertificate: true,
	}
	if err := service.saveServerWithCredential(profile, "super-secret", true); err != nil {
		t.Fatalf("saveServerWithCredential() error = %v", err)
	}
	profile.Name = "vault-renamed"
	if err := service.saveServerWithCredential(profile, "updated-secret", true); err != nil {
		t.Fatalf("saveServerWithCredential() upsert error = %v", err)
	}

	servers, err := service.SavedServers()
	if err != nil {
		t.Fatalf("SavedServers() error = %v", err)
	}
	if len(servers) != 1 || servers[0].Name != "vault-renamed" || servers[0].ID == "" || !servers[0].CredentialStored {
		t.Fatalf("SavedServers() = %#v", servers)
	}
	account := credentialService + "/" + credentialAccount(servers[0].ID, "api_key")
	if credentials.secrets[account] != "updated-secret" {
		t.Fatalf("keychain secret = %q", credentials.secrets[account])
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if strings.Contains(string(payload), "updated-secret") || strings.Contains(string(payload), "super-secret") {
		t.Fatalf("profile file contains secret: %s", payload)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("profile permissions = %o, want 600", got)
	}

	if err := service.DeleteSavedServer(servers[0].ID); err != nil {
		t.Fatalf("DeleteSavedServer() error = %v", err)
	}
	servers, err = service.SavedServers()
	if err != nil || len(servers) != 0 {
		t.Fatalf("SavedServers() after delete = %#v, %v", servers, err)
	}
	if len(credentials.secrets) != 0 {
		t.Fatalf("keychain entries after delete = %#v", credentials.secrets)
	}
}

func TestSavedServersRejectsUnknownVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "servers.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"servers":[]}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	service := &TrueNASService{profilesPath: path}
	if _, err := service.SavedServers(); err == nil {
		t.Fatal("SavedServers() error = nil")
	}
}
