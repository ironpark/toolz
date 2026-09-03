package refstore

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrRefNotFound 는 ref 가 없다는 뜻이다.
	ErrRefNotFound = errors.New("ref not found")
	// ErrLockBusy 는 잠금을 못 잡았다는 뜻이다. 재시도할 수 있다.
	ErrLockBusy = errors.New("ref lock busy")
	// ErrCASConflict 는 남이 먼저 바꿨다는 뜻이다. 다시 판단해야 한다.
	ErrCASConflict = errors.New("ref changed")
)

// classifyRefError 는 git update-ref 의 stderr 를 분류한다 (design §14.6).
//
// 모르는 입력은 반드시 일반 오류로 떨어진다. 잘못된 ErrLockBusy 는 무한 재시도나
// 중복 배정으로 이어지므로, 안전한 방향은 "재시도 가능" 이 아니라 "명시적 실패" 다.
//
// 설계 §14.6 은 lock 검사를 먼저 두었지만 그 순서로는 동작하지 않는다. git 은
// CAS 실패도 "cannot lock ref" 로 감싸서 낸다.
//
//	cannot lock ref 'refs/x': is at <a> but expected <b>
//	cannot lock ref 'refs/x': reference already exists
//
// 두 문자열이 한 메시지에 같이 있으므로, 더 구체적인 CAS 판정을 먼저 해야
// 경쟁 패배가 재시도 가능 오류로 둔갑하지 않는다.
func classifyRefError(stderr []byte, exitCode int) error {
	s := string(stderr)
	switch {
	case strings.Contains(s, "but expected"),
		strings.Contains(s, "reference already exists"):
		return ErrCASConflict
	case strings.Contains(s, "cannot lock ref"),
		strings.Contains(s, "unable to create") && strings.Contains(s, ".lock"),
		strings.Contains(s, "Unable to create") && strings.Contains(s, ".lock"):
		return ErrLockBusy
	}
	if trimmed := strings.TrimSpace(s); trimmed != "" {
		return fmt.Errorf("update-ref: %s", trimmed)
	}
	// stderr 가 비어있는데 exit≠0 인 경우도 일반 오류다.
	return fmt.Errorf("update-ref: exit %d", exitCode)
}
