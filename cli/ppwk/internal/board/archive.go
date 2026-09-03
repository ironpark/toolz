package board

import (
	"errors"
	"fmt"

	"github.com/ironpark/toolz/cli/ppwk/internal/gitobj"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// ErrAlreadyArchived 는 archive/ 에 같은 ID 가 이미 있다는 뜻이다.
var ErrAlreadyArchived = errors.New("이미 archive 에 있습니다")

// ErrNotTerminal 은 종료 상태가 아닌 이슈를 옮기려 했다는 뜻이다.
var ErrNotTerminal = errors.New("종료 상태가 아닙니다")

// Archive 는 종료된 이슈를 issues/ 에서 archive/ 로 옮긴다 (§4.4).
//
// 삭제와 생성을 하나의 update-ref 트랜잭션으로 묶는다. 개별 호출 두 번으로
// 하면 그 사이에 프로세스가 죽었을 때 이슈가 양쪽에 동시에 있거나 양쪽에서
// 사라진다 — 어느 쪽도 복구가 쉽지 않다 (T6.2).
func (b *Board) Archive(id string) (*Issue, error) {
	if err := refstore.ValidateID(id); err != nil {
		return nil, err
	}
	if err := b.requireWritable(); err != nil {
		return nil, err
	}

	source, target := refstore.Issues+id, refstore.Archive+id
	attempts := max(b.backoff.LockAttempts, 1)
	var lastErr error
	for attempt := range attempts {
		hash, err := b.store.Get(source)
		if isNotFound(err) {
			// archive 에 이미 있으면 남이 먼저 옮긴 것이다. 멱등하게 본다 —
			// 자동 이동과 --sweep 이 겹치는 것은 정상적인 일이다.
			if archived, err := b.Show(id); err == nil && archived.Archived() {
				return archived, nil
			}
			return nil, fmt.Errorf("%s: %w", id, ErrNotFound)
		}
		if err != nil {
			return nil, err
		}

		var issue model.Issue
		if _, _, _, err := gitobj.Read(b.repo, hash, gitobj.FileIssue, &issue); err != nil {
			return nil, err
		}
		if !issue.Status.Terminal() {
			return nil, fmt.Errorf("%s(%s): %w", id, issue.Status, ErrNotTerminal)
		}

		// create 는 대상이 없을 때만 성공한다. 같은 ID 를 덮어쓰지 않는 것이
		// 여기서 공짜로 얻어진다.
		err = b.store.Transaction([]refstore.RefOp{
			{Kind: refstore.OpCreate, Ref: target, New: hash},
			{Kind: refstore.OpDelete, Ref: source, Old: hash},
		})
		switch {
		case err == nil:
			return b.Show(id)
		case errors.Is(err, refstore.ErrLockBusy):
			// 트랜잭션은 전부 성공 아니면 전부 실패다. 통째로 다시 한다.
			lastErr = err
			b.backoff.Wait(attempt)
		case errors.Is(err, refstore.ErrCASConflict):
			// 대상이 이미 있거나, source 가 그 사이 바뀌었다. 앞쪽이면 재시도해도
			// 답이 같다 — 덮어쓰지 않는 것이 규칙이므로 즉시 끝낸다.
			if _, getErr := b.store.Get(target); getErr == nil {
				return nil, fmt.Errorf("%s: %w", id, ErrAlreadyArchived)
			}
			lastErr = err
			b.backoff.Wait(attempt)
		default:
			return nil, err
		}
	}
	return nil, &ConflictError{ID: id, Attempts: attempts, Cause: lastErr}
}

// ArchiveSweep 은 종료 상태인데 issues/ 에 남은 이슈를 전부 옮긴다.
//
// 평소에는 done/cancel 이 자동으로 옮기므로 복구용이다. 자동 이동이 실패해도
// 전이 자체는 성공한 상태로 남으므로, 그 잔재를 걷어내는 것이 이 명령이다.
func (b *Board) ArchiveSweep() ([]*Issue, error) {
	entries, err := b.List(ListOptions{
		Status: []model.Status{model.StatusDone, model.StatusCancelled},
	})
	if err != nil {
		return nil, err
	}
	var moved []*Issue
	for _, entry := range entries {
		issue, err := b.Archive(entry.ID)
		if err != nil {
			// 하나가 경쟁에 밀려도 나머지를 계속 옮긴다.
			var conflict *ConflictError
			if errors.As(err, &conflict) || errors.Is(err, ErrNotFound) ||
				errors.Is(err, ErrAlreadyArchived) || errors.Is(err, ErrNotTerminal) {
				continue
			}
			return moved, err
		}
		moved = append(moved, issue)
	}
	return moved, nil
}

// archiveAfterTransition 은 종료 상태가 된 이슈를 뒤이어 옮긴다.
//
// 실패해도 전이 오류로 올리지 않는다. 이슈는 이미 done 이고 그 사실은
// issues/ 에 정확히 기록돼 있다. 위치가 늦게 정리되는 것과 done 이 실패했다고
// 보고하는 것 중에서는 전자가 훨씬 덜 위험하다 — 후자는 에이전트가 done 을
// 다시 시도하게 만들고, 그것은 exit 3 이 된다. 남은 잔재는 fsck 와
// archive --sweep 이 걷는다.
func (b *Board) archiveAfterTransition(issue *Issue) *Issue {
	if !issue.Status.Terminal() {
		return issue
	}
	if archived, err := b.Archive(issue.ID); err == nil {
		return archived
	}
	return issue
}
