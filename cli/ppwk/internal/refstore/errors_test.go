package refstore

import (
	"errors"
	"testing"
)

// T0.8~T0.10 classifyRefError
func TestClassifyRefError(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   error
	}{
		{
			// T0.9 CAS 실패는 lock 문구를 달고 오지만 CAS 로 분류돼야 한다.
			name:   "CAS 불일치",
			stderr: "fatal: update_ref failed for ref 'refs/probe': cannot lock ref 'refs/probe': is at 95db5d39 but expected c6e0b764",
			want:   ErrCASConflict,
		},
		{
			name:   "이미 존재",
			stderr: "fatal: update_ref failed for ref 'refs/probe': cannot lock ref 'refs/probe': reference already exists",
			want:   ErrCASConflict,
		},
		{
			// T0.8 순수한 잠금 실패
			name:   "잠금 획득 실패",
			stderr: "fatal: cannot lock ref 'refs/ppwk/issues/T001': Unable to create '/repo/.git/refs/ppwk/issues/T001.lock': File exists.",
			want:   ErrLockBusy,
		},
		{
			name:   "소문자 unable to create",
			stderr: "error: unable to create '/repo/.git/refs/x.lock': Permission denied",
			want:   ErrLockBusy,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyRefError([]byte(tt.stderr), 128)
			if !errors.Is(got, tt.want) {
				t.Fatalf("classifyRefError() = %v, want %v", got, tt.want)
			}
		})
	}
}

// T0.10 알 수 없는 문자열은 일반 오류다 — 절대 ErrLockBusy 가 아니다.
func TestClassifyRefErrorUnknown(t *testing.T) {
	inputs := []string{
		"fatal: not a git repository",
		"",
		"error: something went sideways",
	}
	for _, in := range inputs {
		got := classifyRefError([]byte(in), 128)
		if got == nil {
			t.Fatalf("classifyRefError(%q) = nil, want error", in)
		}
		if errors.Is(got, ErrLockBusy) || errors.Is(got, ErrCASConflict) {
			t.Fatalf("classifyRefError(%q) = %v, want 일반 오류", in, got)
		}
	}
}

// stderr 가 비어 있는데 exit≠0 이면 일반 오류다.
func TestClassifyRefErrorEmptyStderr(t *testing.T) {
	got := classifyRefError(nil, 1)
	if got == nil {
		t.Fatal("classifyRefError() = nil, want error")
	}
	if errors.Is(got, ErrLockBusy) {
		t.Fatalf("classifyRefError() = %v, want 일반 오류", got)
	}
}
