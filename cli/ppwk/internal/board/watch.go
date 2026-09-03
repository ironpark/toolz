package board

import (
	"context"
	"strings"
	"time"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
	"github.com/ironpark/toolz/cli/ppwk/internal/watch"
)

// WatchOptions 는 감지 설정이다 (features §6).
type WatchOptions struct {
	// Interval 은 polling 주기다. 0 이면 2초다.
	Interval time.Duration
	// Filter 는 볼 ref prefix 다. 빈 문자열이면 refs/ppwk/ 전체다.
	Filter string
}

// Poller 는 이 보드를 보는 poller 를 만든다. 한 주기만 돌려보는 테스트용이다.
func (b *Board) Poller(opts WatchOptions) *watch.Poller {
	return &watch.Poller{Lister: b.store, Prefix: opts.Filter}
}

// Watch 는 변경을 감지해 emit 을 부른다. ctx 가 끝날 때까지 돈다.
//
// 조회만 하므로 세션을 등록하지 않는다. watch 는 몇 개든 동시에 띄울 수
// 있어야 하고, worktree 배타의 대상이 아니다 (features §8.4).
func (b *Board) Watch(ctx context.Context, opts WatchOptions, emit func(watch.Event) error) error {
	interval := opts.Interval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	poller := b.Poller(opts)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := b.pollOnce(poller, emit); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			// SIGINT 는 오류가 아니다. 정리하고 조용히 끝낸다.
			return nil
		case <-ticker.C:
		}
	}
}

// pollOnce 는 한 주기를 돌고 이벤트를 살찌워 내보낸다.
func (b *Board) pollOnce(poller *watch.Poller, emit func(watch.Event) error) error {
	events, err := poller.Poll()
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := emit(b.enrich(event)); err != nil {
			return err
		}
	}
	return nil
}

// enrich 는 이벤트에 이슈 ID 와 상태를 붙인다.
//
// 상태는 trailer 에서 읽는다. issue.json 을 열지 않는 이유는 목록과 같다 —
// 이벤트가 쏟아질 때 tree 를 매번 여는 비용을 치를 이유가 없다 (§5.1).
func (b *Board) enrich(event watch.Event) watch.Event {
	event.ID = issueIDFromRef(event.Ref)
	if event.ID == "" || event.New == "" {
		// 삭제된 ref 에는 읽을 상태가 없다. ID 만으로 충분하다.
		return event
	}
	entry, err := b.readEntry(event.Ref, plumbing.NewHash(event.New))
	if err != nil {
		return event
	}
	event.Status = string(entry.Status)
	return event
}

// issueIDFromRef 는 이슈 ref 에서 ID 를 뽑는다. 이슈가 아니면 빈 문자열이다.
func issueIDFromRef(ref string) string {
	for _, prefix := range []string{refstore.Issues, refstore.Archive} {
		if id, found := strings.CutPrefix(ref, prefix); found {
			return id
		}
	}
	return ""
}
