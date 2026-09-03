package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/ironpark/toolz/cli/ppwk/internal/board"
	"github.com/urfave/cli/v3"
)

// 종료 코드 (features §0.3)
const (
	ExitOK           = 0 // 성공
	ExitGeneral      = 1 // 일반 오류
	ExitUsage        = 2 // 사용법 오류
	ExitTransition   = 3 // 전이 규칙 위반
	ExitCASConflict  = 4 // CAS 경쟁 실패
	ExitNotFound     = 5 // 대상 없음
	ExitSchemaVerErr = 6 // 스키마 버전 불일치
)

// Error 는 종료 코드를 동반하는 오류다. urfave/cli 의 ExitCoder 를 만족한다.
type Error struct {
	Code int
	Kind string
	Msg  string
}

func (e *Error) Error() string { return e.Msg }
func (e *Error) ExitCode() int { return e.Code }

func newError(code int, kind, format string, args ...any) *Error {
	return &Error{Code: code, Kind: kind, Msg: fmt.Sprintf(format, args...)}
}

// UsageError 는 잘못된 인자 조합에 쓴다.
func UsageError(format string, args ...any) *Error {
	return newError(ExitUsage, "usage", format, args...)
}

// NotFoundError 는 대상 이슈/plan/결정이 없을 때 쓴다.
func NotFoundError(format string, args ...any) *Error {
	return newError(ExitNotFound, "not_found", format, args...)
}

// TransitionError 는 허용되지 않는 상태 전이에 쓴다.
func TransitionError(format string, args ...any) *Error {
	return newError(ExitTransition, "invalid_transition", format, args...)
}

// CASConflictError 는 경쟁에서 밀렸을 때 쓴다. 재시도 가능 신호다.
func CASConflictError(format string, args ...any) *Error {
	return newError(ExitCASConflict, "cas_conflict", format, args...)
}

// notImplemented 는 아직 구현되지 않은 명령의 자리표시자다.
func notImplemented(name string) error {
	return newError(ExitGeneral, "not_implemented", "%s: 아직 구현되지 않았습니다", name)
}

// action 은 도메인 함수를 cli.Command 의 Action 으로 감싼다.
//
// 종료 코드 결정을 여기 한 곳에 모은다. 명령마다 오류를 분류하면 새 명령이
// 늘 때마다 빠뜨리게 되고, 그 결과는 조용히 잘못된 종료 코드다.
func action(fn func(*ctx) error) func(context.Context, *cli.Command) error {
	return func(_ context.Context, c *cli.Command) error {
		return classify(fn(newCtx(c)))
	}
}

// classify 는 도메인 오류를 종료 코드에 맞춘다 (features §0.3).
func classify(err error) error {
	if err == nil {
		return nil
	}
	// 이미 코드가 붙어 있으면 그대로 둔다.
	var coded *Error
	if errors.As(err, &coded) {
		return err
	}

	var transition *board.TransitionError
	if errors.As(err, &transition) {
		return TransitionError("%v", err)
	}
	var conflict *board.ConflictError
	if errors.As(err, &conflict) {
		return CASConflictError("%v", err)
	}
	switch {
	// 이 둘은 규칙 위반이다. 재시도해도 답이 같으므로 exit 4 가 아니다.
	case errors.Is(err, board.ErrNotTerminal), errors.Is(err, board.ErrAlreadyArchived),
		errors.Is(err, board.ErrPhaseInUse):
		return TransitionError("%v", err)
	case errors.Is(err, board.ErrNotFound):
		return NotFoundError("%v", err)
	case errors.Is(err, board.ErrSchemaTooNew):
		return newError(ExitSchemaVerErr, "schema_mismatch", "%v", err)
	}
	return err
}
