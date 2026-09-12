package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ApplyDiagnosticTuning changes only the selected property after a fresh read.
func (s *TrueNASService) ApplyDiagnosticTuning(dataset, action, expectedEndpoint string) error {
	s.mu.RLock()
	client, endpoint := s.client, s.endpoint
	s.mu.RUnlock()
	if client == nil || expectedEndpoint == "" || endpoint != expectedEndpoint {
		return errors.New("서버 연결이 변경되었습니다. 대상 서버에서 다시 점검해 주세요")
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	return applyDiagnosticTuning(ctx, client, dataset, action)
}

func applyDiagnosticTuning(ctx context.Context, client apiCaller, dataset, action string) error {
	property, previous, next := "", "", ""
	switch action {
	case "compression_lz4":
		property, previous, next = "compression", "OFF", "LZ4"
	case "sync_standard":
		property, previous, next = "sync", "DISABLED", "STANDARD"
	default:
		return errors.New("지원하지 않는 자동 조치입니다")
	}
	parts := strings.Split(dataset, "/")
	if len(parts) < 2 {
		return errors.New("풀 루트는 자동 튜닝 대상이 아닙니다")
	}
	for _, part := range parts {
		if part == "" || strings.HasPrefix(part, ".") || part == "ix-apps" || part == "ix-applications" || strings.ContainsAny(part, "@#") {
			return errors.New("시스템 또는 올바르지 않은 데이터셋은 자동 튜닝할 수 없습니다")
		}
	}
	var current apiStorageDataset
	if err := client.Call(ctx, "pool.dataset.query", []any{[][]any{{"id", "=", dataset}}, map[string]any{"get": true}}, &current); err != nil {
		return fmt.Errorf("적용 전 데이터셋 조회 실패: %w", err)
	}
	if current.ID != dataset || current.Type != "FILESYSTEM" || current.Locked {
		return errors.New("잠금 해제된 파일시스템 데이터셋만 자동 튜닝할 수 있습니다")
	}
	value := current.Compression.Value
	if property == "sync" {
		value = current.Sync.Value
	}
	if !strings.EqualFold(value, previous) {
		return errors.New("점검 이후 설정이 변경되었습니다. 다시 점검해 주세요")
	}
	if err := client.Call(ctx, "pool.dataset.update", []any{dataset, map[string]any{property: next}}, nil); err != nil {
		return fmt.Errorf("자동 조치 실패: %w", err)
	}
	return nil
}
