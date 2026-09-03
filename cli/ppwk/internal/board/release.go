package board

import (
	"errors"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
)

// ReleaseMine 은 이 세션이 보유한 이슈를 전부 반납한다 (features §3, §1.3).
//
// claimed 만 건드린다. working 은 미커밋 작업이 있을 수 있어 손대지 않는다
// (D15) — 오케스트레이터가 세션을 끝냈다는 것이 작업이 끝났다는 뜻은 아니다.
func (b *Board) ReleaseMine(opts TransitionOptions) ([]*Issue, error) {
	entries, err := b.List(ListOptions{
		Status: []model.Status{model.StatusClaimed},
		Owner:  b.identity.Agent,
	})
	if err != nil {
		return nil, err
	}

	var released []*Issue
	for _, entry := range entries {
		issue, err := b.Transition(ActionRelease, entry.ID, opts)
		if err != nil {
			// 그 사이 남이 가져갔거나 상태가 바뀐 것은 정상이다. 나머지를
			// 계속 반납한다 — 하나 때문에 전부 멈추면 세션 정리가 실패한다.
			var transition *TransitionError
			var conflict *ConflictError
			if errors.As(err, &transition) || errors.As(err, &conflict) || errors.Is(err, ErrNotFound) {
				continue
			}
			return released, err
		}
		released = append(released, issue)
	}
	return released, nil
}
