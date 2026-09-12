package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// SetServiceStartup changes only the boot policy, never the running state.
func (s *TrueNASService) SetServiceStartup(name string, automatic bool) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	return setServiceStartup(ctx, client, name, automatic)
}

func setServiceStartup(ctx context.Context, client apiCaller, name string, automatic bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("서비스 이름이 필요합니다")
	}
	if err := client.Call(ctx, "service.update", []any{name, map[string]any{"enable": automatic}}, nil); err != nil {
		return fmt.Errorf("서비스 시작 설정 변경 실패: %w", err)
	}
	return nil
}
