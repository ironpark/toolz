package main

import (
	"context"
	"fmt"
)

// HTTPSRedirect returns the current web UI redirect setting.
func (s *TrueNASService) HTTPSRedirect() (bool, error) {
	client, err := s.connectedClient()
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	var config struct {
		Enabled bool `json:"ui_httpsredirect"`
	}
	if err := client.Call(ctx, "system.general.config", nil, &config); err != nil {
		return false, fmt.Errorf("HTTPS 리다이렉션 설정 조회 실패: %w", err)
	}
	return config.Enabled, nil
}

// SetHTTPSRedirect saves the setting and schedules the web UI restart.
func (s *TrueNASService) SetHTTPSRedirect(enabled bool) error {
	client, err := s.connectedClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	if err := client.Call(ctx, "system.general.update", []any{map[string]any{
		"ui_httpsredirect": enabled,
		"ui_restart_delay": 3,
	}}, nil); err != nil {
		return fmt.Errorf("HTTPS 리다이렉션 설정 저장 실패: %w", err)
	}
	return nil
}
