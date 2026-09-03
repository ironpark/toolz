package main

import (
	"errors"
	"fmt"

	keyring "github.com/zalando/go-keyring"
)

const credentialService = "CharmTrue TrueNAS"

type credentialStore interface {
	Set(service, account, secret string) error
	Get(service, account string) (string, error)
	Delete(service, account string) error
}

type systemCredentialStore struct{}

func (systemCredentialStore) Set(service, account, secret string) error {
	return keyring.Set(service, account, secret)
}

func (systemCredentialStore) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}

func (systemCredentialStore) Delete(service, account string) error {
	return keyring.Delete(service, account)
}

func (s *TrueNASService) credentialStore() credentialStore {
	if s.credentials != nil {
		return s.credentials
	}
	return systemCredentialStore{}
}

func credentialAccount(serverID, authenticationMethod string) string {
	return serverID + ":" + authenticationMethod
}

func alternateAuthenticationMethod(authenticationMethod string) string {
	if authenticationMethod == "password" {
		return "api_key"
	}
	return "password"
}

func credentialReadError(err error) error {
	switch {
	case errors.Is(err, keyring.ErrNotFound):
		return errors.New("키체인에서 로그인 정보를 찾지 못했습니다. 다시 입력해 주세요")
	case errors.Is(err, keyring.ErrUnsupportedPlatform):
		return errors.New("이 환경에서는 운영체제 키체인을 사용할 수 없습니다")
	default:
		return fmt.Errorf("키체인 로그인 정보를 읽지 못했습니다: %w", err)
	}
}

func credentialWriteError(err error) error {
	if errors.Is(err, keyring.ErrUnsupportedPlatform) {
		return errors.New("이 환경에서는 운영체제 키체인을 사용할 수 없습니다")
	}
	return fmt.Errorf("키체인에 로그인 정보를 저장하지 못했습니다: %w", err)
}

func isCredentialNotFound(err error) bool {
	return errors.Is(err, keyring.ErrNotFound)
}
