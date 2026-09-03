package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const savedServersVersion = 1

// SavedServer is a reusable TrueNAS connection profile. Credentials are
// deliberately excluded so passwords and API keys are never written to disk.
type SavedServer struct {
	ID                      string `json:"id"`
	Name                    string `json:"name"`
	Endpoint                string `json:"endpoint"`
	Username                string `json:"username"`
	AuthenticationMethod    string `json:"authenticationMethod"`
	AllowPrivateCertificate bool   `json:"allowPrivateCertificate"`
	CredentialStored        bool   `json:"credentialStored"`
	LastConnected           string `json:"lastConnected"`
}

type savedServersFile struct {
	Version int           `json:"version"`
	Servers []SavedServer `json:"servers"`
}

// SavedServers returns connection profiles without exposing credentials.
func (s *TrueNASService) SavedServers() ([]SavedServer, error) {
	s.profilesMu.Lock()
	defer s.profilesMu.Unlock()
	path, err := s.savedServersPath()
	if err != nil {
		return nil, err
	}
	servers, err := readSavedServers(path)
	if err != nil {
		return nil, fmt.Errorf("저장된 서버 목록을 읽지 못했습니다: %w", err)
	}
	return servers, nil
}

// DeleteSavedServer removes one connection profile. It does not disconnect an
// active session because saved profiles and live connections are independent.
func (s *TrueNASService) DeleteSavedServer(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("삭제할 서버 ID가 필요합니다")
	}
	s.profilesMu.Lock()
	defer s.profilesMu.Unlock()
	path, err := s.savedServersPath()
	if err != nil {
		return err
	}
	servers, err := readSavedServers(path)
	if err != nil {
		return fmt.Errorf("저장된 서버 목록을 읽지 못했습니다: %w", err)
	}
	filtered := make([]SavedServer, 0, len(servers))
	found := false
	for _, server := range servers {
		if server.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, server)
	}
	if !found {
		return errors.New("저장된 서버를 찾을 수 없습니다")
	}
	if err := s.deleteServerCredential(id, servers); err != nil {
		return err
	}
	if err := writeSavedServers(path, filtered); err != nil {
		return fmt.Errorf("서버 프로필을 삭제하지 못했습니다: %w", err)
	}
	return nil
}

func (s *TrueNASService) savedServer(id string) (SavedServer, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return SavedServer{}, errors.New("서버 ID가 필요합니다")
	}
	servers, err := s.SavedServers()
	if err != nil {
		return SavedServer{}, err
	}
	for _, server := range servers {
		if server.ID == id {
			return server, nil
		}
	}
	return SavedServer{}, errors.New("저장된 서버를 찾을 수 없습니다")
}

func (s *TrueNASService) saveServerWithCredential(server SavedServer, secret string, saveCredential bool) error {
	server.ID = savedServerID(server.Endpoint, server.Username)
	store := s.credentialStore()
	account := credentialAccount(server.ID, server.AuthenticationMethod)
	alternateAccount := credentialAccount(server.ID, alternateAuthenticationMethod(server.AuthenticationMethod))
	if saveCredential {
		if err := store.Set(credentialService, account, secret); err != nil {
			return credentialWriteError(err)
		}
		server.CredentialStored = true
		if err := store.Delete(credentialService, alternateAccount); err != nil && !isCredentialNotFound(err) {
			// An obsolete credential is inaccessible and harmless; the current
			// credential remains the only reference stored in the profile.
		}
	} else {
		servers, err := s.SavedServers()
		if err != nil {
			return err
		}
		for _, saved := range servers {
			if saved.ID == server.ID && saved.CredentialStored {
				for _, credentialAccount := range []string{account, alternateAccount} {
					if err := store.Delete(credentialService, credentialAccount); err != nil && !isCredentialNotFound(err) {
						return credentialWriteError(err)
					}
				}
				break
			}
		}
		server.CredentialStored = false
	}
	if err := s.saveServer(server); err != nil {
		if saveCredential {
			_ = store.Delete(credentialService, account)
		}
		return err
	}
	return nil
}

func (s *TrueNASService) deleteServerCredential(id string, servers []SavedServer) error {
	store := s.credentialStore()
	for _, server := range servers {
		if server.ID != id {
			continue
		}
		if !server.CredentialStored {
			return nil
		}
		for _, authenticationMethod := range []string{"api_key", "password"} {
			err := store.Delete(credentialService, credentialAccount(id, authenticationMethod))
			if err != nil && !isCredentialNotFound(err) {
				return credentialWriteError(err)
			}
		}
		return nil
	}
	return nil
}

func (s *TrueNASService) saveServer(server SavedServer) error {
	server.Endpoint = strings.TrimSpace(server.Endpoint)
	server.Username = strings.TrimSpace(server.Username)
	server.AuthenticationMethod = strings.TrimSpace(server.AuthenticationMethod)
	server.Name = strings.TrimSpace(server.Name)
	if server.Endpoint == "" || server.Username == "" {
		return errors.New("서버 주소와 사용자명이 필요합니다")
	}
	if server.AuthenticationMethod != "api_key" && server.AuthenticationMethod != "password" {
		return errors.New("지원하지 않는 인증 방식입니다")
	}
	if server.Name == "" {
		server.Name = server.Endpoint
	}
	server.ID = savedServerID(server.Endpoint, server.Username)
	server.LastConnected = time.Now().UTC().Format(time.RFC3339)

	s.profilesMu.Lock()
	defer s.profilesMu.Unlock()
	path, err := s.savedServersPath()
	if err != nil {
		return err
	}
	servers, err := readSavedServers(path)
	if err != nil {
		return err
	}
	updatedServers := make([]SavedServer, 0, len(servers)+1)
	updatedServers = append(updatedServers, server)
	for _, saved := range servers {
		if saved.ID != server.ID {
			updatedServers = append(updatedServers, saved)
		}
	}
	return writeSavedServers(path, updatedServers)
}

func (s *TrueNASService) savedServersPath() (string, error) {
	if s.profilesPath != "" {
		return s.profilesPath, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("사용자 설정 디렉터리를 찾지 못했습니다: %w", err)
	}
	return filepath.Join(configDir, "CharmTrue", "servers.json"), nil
}

func savedServerID(endpoint, username string) string {
	digest := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(endpoint)) + "\x00" + strings.ToLower(strings.TrimSpace(username))))
	return hex.EncodeToString(digest[:12])
}

func readSavedServers(path string) ([]SavedServer, error) {
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []SavedServer{}, nil
	}
	if err != nil {
		return nil, err
	}
	var file savedServersFile
	if err := json.Unmarshal(payload, &file); err != nil {
		return nil, err
	}
	if file.Version != savedServersVersion {
		return nil, fmt.Errorf("지원하지 않는 서버 프로필 버전 %d", file.Version)
	}
	if file.Servers == nil {
		return []SavedServer{}, nil
	}
	return file.Servers, nil
}

func writeSavedServers(path string, servers []SavedServer) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(savedServersFile{Version: savedServersVersion, Servers: servers}, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".servers-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
