package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ironpark/toolz/cli/ppwk/internal/board"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/urfave/cli/v3"
)

// 도메인 오류가 features §0.3 의 종료 코드로 옮겨져야 한다.
func TestClassifyExitCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code int
		kind string
	}{
		{"전이 위반", &board.TransitionError{ID: "T001", From: model.StatusWorking, To: model.StatusWorking}, ExitTransition, "invalid_transition"},
		{"경쟁 상한", &board.ConflictError{ID: "T001", Attempts: 8, Cause: errors.New("x")}, ExitCASConflict, "cas_conflict"},
		{"없음", fmt.Errorf("T404: %w", board.ErrNotFound), ExitNotFound, "not_found"},
		{"스키마", fmt.Errorf("%w: 너무 높음", board.ErrSchemaTooNew), ExitSchemaVerErr, "schema_mismatch"},
		{"이미 분류됨", UsageError("잘못된 인자"), ExitUsage, "usage"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classify(tt.err)
			var coded *Error
			if !errors.As(got, &coded) {
				t.Fatalf("classify() = %v, 종료 코드가 붙지 않았습니다", got)
			}
			if coded.Code != tt.code || coded.Kind != tt.kind {
				t.Fatalf("code=%d kind=%q, want %d %q", coded.Code, coded.Kind, tt.code, tt.kind)
			}
			// ExitCoder 로도 읽혀야 한다. main 이 그 경로로 코드를 정한다.
			var exiter cli.ExitCoder
			if !errors.As(got, &exiter) || exiter.ExitCode() != tt.code {
				t.Fatalf("ExitCoder 로 읽히지 않습니다: %v", got)
			}
		})
	}

	// 모르는 오류는 손대지 않는다. 억지로 코드를 붙이면 exit 1 이어야 할 것이
	// 재시도 가능 신호로 둔갑한다.
	plain := errors.New("무슨 일인지 모름")
	if got := classify(plain); got != plain {
		t.Fatalf("classify(plain) = %v, 원본 그대로여야 합니다", got)
	}
	if classify(nil) != nil {
		t.Fatal("classify(nil) 이 nil 이 아닙니다")
	}
}
