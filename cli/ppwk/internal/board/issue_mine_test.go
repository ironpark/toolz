package board

import (
	"testing"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/session"
)

// E2E-1b/E2E-22b 의 전제 — list --mine 은 owner 와 session 이 둘 다 맞는
// 것만 돌려준다.
//
// owner 만 보면 같은 이름으로 재시작한 이전 세션의 이슈가 딸려 온다. 그것은
// 회수 대상이지 내 것이 아니다 (§3.6).
func TestListMineMatchesAgentAndSession(t *testing.T) {
	b, dir := initBoardDir(t)
	mine := issueIn(t, b, model.StatusClaimed)

	// 같은 에이전트, 다른 세션.
	other, err := Open(dir, session.Identity{Agent: b.identity.Agent, Session: "old-session"})
	if err != nil {
		t.Fatal(err)
	}
	other.allowSharedWorktree = true
	stale := issueIn(t, other, model.StatusClaimed)

	// 다른 에이전트.
	third, err := Open(dir, session.Identity{Agent: "agent-b", Session: "sb"})
	if err != nil {
		t.Fatal(err)
	}
	third.allowSharedWorktree = true
	foreign := issueIn(t, third, model.StatusClaimed)

	entries, err := b.List(ListOptions{Mine: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != mine.ID {
		t.Fatalf("--mine = %v, want [%s] (제외 대상 %s, %s)",
			entries, mine.ID, stale.ID, foreign.ID)
	}
}
