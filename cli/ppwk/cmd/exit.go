package cmd

import "fmt"

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
