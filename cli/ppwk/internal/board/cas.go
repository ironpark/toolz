package board

import (
	"errors"
	"fmt"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/ironpark/toolz/cli/ppwk/internal/gitobj"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// TransitionError 는 전이 규칙 위반이다 (§7.3, exit 3).
//
// 재시도하지 않는다. 다시 읽어도 규칙은 그대로이고, 상태가 바뀌었다면 그것은
// CAS 실패로 잡힌다. 여기서 재시도하면 사용자 오류를 무한 루프로 바꾸는 셈이다.
type TransitionError struct {
	ID     string
	From   model.Status
	To     model.Status
	Reason string
}

func (e *TransitionError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("%s: %s → %s 전이가 허용되지 않습니다: %s", e.ID, e.From, e.To, e.Reason)
	}
	return fmt.Sprintf("%s: %s → %s 전이가 허용되지 않습니다", e.ID, e.From, e.To)
}

// ConflictError 는 재시도 상한에 닿았다는 뜻이다 (§7.3, exit 4).
//
// 실패지만 사용자 잘못이 아니다. 같은 명령을 다시 실행하면 될 가능성이 높으므로
// 종료 코드를 분리해 두었다 — 호출하는 쪽이 재시도할지 판단할 수 있다.
type ConflictError struct {
	ID       string
	Attempts int
	Cause    error
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%s: %d번 시도했지만 경쟁에서 밀렸습니다: %v", e.ID, e.Attempts, e.Cause)
}

func (e *ConflictError) Unwrap() error { return e.Cause }

// Mutation 은 이슈 하나에 가하는 변경이다.
type Mutation struct {
	// ID 는 대상 이슈다.
	ID string
	// Event 는 commit subject 의 앞머리다. git log 가 곧 history 다 (§3.3).
	Event string
	// Message 는 subject 에 덧붙일 사유다 (--message).
	Message string
	// Apply 는 현재 상태를 받아 다음 상태로 바꾼다.
	//
	// 전이 규칙 검사도 여기서 한다 (§4.1 3단계). CAS 에 밀리면 다시 읽은
	// 상태로 이 함수가 한 번 더 불린다. 그러므로 Apply 는 바깥 상태를 건드리지
	// 않고 인자만 보고 판단해야 한다 — 여러 번 불릴 수 있다.
	Apply func(issue *model.Issue) error
}

// Mutate 는 §4.1 의 CAS 루프다. 모든 상태 변경이 예외 없이 이 경로를 탄다.
//
//  1. old 읽기
//  2. 현재 상태 파싱
//  3. 전이 규칙 검사
//  4. 새 객체 생성 (Agent-Session trailer 포함)
//  5. CAS
//  6. lock 실패 → 재시도 / CAS 실패 → 처음부터
//
// 객체를 먼저 만들고 ref 를 나중에 바꾸는 순서가 이 설계의 안전장치다. 4단계와
// 5단계 사이 어느 지점에서 죽어도 dangling commit 만 남고 ref 는 그대로다 —
// 부분 상태가 생기지 않는다.
func (b *Board) Mutate(m Mutation) (*Issue, error) {
	if err := refstore.ValidateID(m.ID); err != nil {
		return nil, err
	}
	if err := b.requireWritable(); err != nil {
		return nil, err
	}

	ref := refstore.Issues + m.ID
	attempts := max(b.backoff.CASAttempts, 1)
	var lastErr error

	for attempt := range attempts {
		// 1~2. 매 회차마다 다시 읽는다. 이전 회차의 판단은 이미 무효다.
		old, current, body, archived, err := b.loadIssue(ref, m.ID)
		if err != nil {
			return nil, err
		}

		// 3. 전이 규칙. 위반이면 즉시 끝난다.
		next := current
		if err := m.Apply(&next); err != nil {
			return nil, err
		}
		// 여기 도달했다는 것은 규칙상 허용되는 변경이라는 뜻이다. 그래도
		// archive 된 이슈는 쓰지 않는다. 되살리려면 이력 정합성 판단이
		// 필요하고, 그것은 도구가 대신할 수 있는 판단이 아니다 (features §7).
		if archived {
			return nil, &TransitionError{ID: m.ID, From: current.Status, To: next.Status,
				Reason: "archive 된 이슈는 수정할 수 없습니다"}
		}
		next.UpdatedAt = model.Now()
		next.UpdatedBy = b.identity.Agent
		// session 은 여기서 건드리지 않는다. 그것은 "누가 마지막에 손댔나"
		// (updated_by) 가 아니라 "어느 세션이 이 이슈를 쥐고 있나" 이며,
		// 소유권과 함께 움직여야 --mine 필터가 반납된 이슈를 놓아준다.
		if err := next.Validate(); err != nil {
			return nil, err
		}

		// 4. 객체 생성. parent 는 방금 읽은 old 다.
		hash, err := b.writeIssueCommitWithMessage(next, body, m.Event, m.Message, old)
		if err != nil {
			return nil, err
		}

		// 5~6.
		err = b.casWithLockRetry(ref, hash, old)
		switch {
		case err == nil:
			return &Issue{Issue: next, Body: body, Ref: ref, Commit: hash}, nil
		case errors.Is(err, refstore.ErrCASConflict), errors.Is(err, refstore.ErrLockBusy):
			// 남이 먼저 바꿨다. 1단계로 돌아간다.
			lastErr = err
			b.backoff.Wait(attempt)
		default:
			return nil, err
		}
	}
	return nil, &ConflictError{ID: m.ID, Attempts: attempts, Cause: lastErr}
}

// casWithLockRetry 는 잠금 실패에만 같은 commit 으로 재시도한다 (§4.2).
//
// 잠금 실패는 "다른 프로세스가 쓰는 중" 이지 "상태가 바뀌었다" 가 아니다.
// 상태가 그대로이므로 만들어 둔 commit 이 그대로 유효하고, 다시 읽고 다시
// 만드는 비용을 치를 이유가 없다. 이 구분을 뭉개면 lock 실패마다 전체
// 재계산을 하거나, 반대로 CAS 실패를 재시도로 오인해 무한 루프에 빠진다.
func (b *Board) casWithLockRetry(ref string, new, old plumbing.Hash) error {
	attempts := max(b.backoff.LockAttempts, 1)
	var err error
	for attempt := range attempts {
		err = b.store.CAS(ref, new, old)
		if !errors.Is(err, refstore.ErrLockBusy) {
			return err
		}
		b.backoff.Wait(attempt)
	}
	return err
}

// loadIssue 는 ref 가 가리키는 현재 상태를 읽는다 (§4.1 1~2단계).
//
// issues/ 에 없으면 archive/ 도 본다. 읽기만 하기 위해서다 — 그래야 종료된
// 이슈에 전이를 시도했을 때 "없는 이슈" 가 아니라 "이미 끝난 이슈" 라고
// 말할 수 있다. 둘은 사용자에게 전혀 다른 사실이다.
func (b *Board) loadIssue(ref, id string) (hash plumbing.Hash, issue model.Issue, body []byte, archived bool, err error) {
	hash, err = b.store.Get(ref)
	if isNotFound(err) {
		hash, err = b.store.Get(refstore.Archive + id)
		if isNotFound(err) {
			// CAS 직전에 ref 가 삭제된 경우도 여기로 온다. 재시도가 아니라 "없음" 이다.
			return plumbing.ZeroHash, model.Issue{}, nil, false, fmt.Errorf("%s: %w", id, ErrNotFound)
		}
		archived = true
	}
	if err != nil {
		return plumbing.ZeroHash, model.Issue{}, nil, false, err
	}
	body, _, _, err = gitobj.Read(b.repo, hash, gitobj.FileIssue, &issue)
	if err != nil {
		return plumbing.ZeroHash, model.Issue{}, nil, false, err
	}
	return hash, issue, body, archived, nil
}
