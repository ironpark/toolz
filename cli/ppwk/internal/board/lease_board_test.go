package board

import (
	"errors"
	"testing"
	"time"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/session"
)

// T4.1 — 초기화 명령 없이 claim 하면 세션이 암묵적으로 등록된다 (§3.6).
//
// 이 설계의 핵심이다. 에이전트가 `session begin` 같은 것을 부르지 않아도
// 회수가 동작해야 하므로, 등록은 첫 쓰기에 딸려 와야 한다.
func TestClaimRegistersSessionImplicitly(t *testing.T) {
	b := initBoard(t)
	if len(b.Agents()) != 0 {
		t.Fatalf("쓰기 전에 이미 기록이 있다: %v", b.Agents())
	}

	issueIn(t, b, model.StatusClaimed)

	leases := b.Agents()
	if len(leases) != 1 {
		t.Fatalf("claim 뒤 기록 %d개, 1개여야 합니다: %v", len(leases), leases)
	}
	if leases[0].Agent != b.Identity().Agent || leases[0].Session != b.Identity().Session {
		t.Fatalf("기록 = %+v, want %+v", leases[0], b.Identity())
	}
}

// T4.16b — 상태 변경은 last_activity 를 갱신한다 (§3.6).
//
// 갱신되지 않으면 일하는 중인 에이전트가 8시간 뒤 죽은 것으로 판정되어 자기
// 이슈를 빼앗긴다. T4.16d(조회는 갱신하지 않는다)의 반대쪽이다.
func TestStateChangeRefreshesActivity(t *testing.T) {
	b := initBoard(t)

	// 시계를 과거에 두고 첫 등록을 만든다. 두 전이의 간격에 기대면
	// 타임스탬프 해상도에 따라 같은 값이 나올 수 있다.
	past := time.Now().Add(-time.Hour)
	b.leases.Now = func() time.Time { return past }
	issue := issueIn(t, b, model.StatusClaimed)
	before := b.Agents()[0].LastActivity

	b.leases.Now = time.Now
	if _, err := b.Transition(ActionStart, issue.ID, TransitionOptions{}); err != nil {
		t.Fatal(err)
	}

	after := b.Agents()[0].LastActivity
	if !after.After(before.Time) {
		t.Fatalf("상태 변경이 last_activity 를 갱신하지 않았다: %s → %s", before, after)
	}
}

// T4.8 — 같은 worktree 를 살아 있는 다른 세션이 쥐고 있으면 상태 변경을
// 거부한다 (§3.6).
//
// 한 작업 트리에서 두 에이전트가 동시에 편집하면 서로의 파일을 덮어쓴다.
// ref 는 CAS 가 지키지만 작업 트리는 아무도 지키지 않으므로 여기서 막는다.
func TestSecondAgentInSameWorktreeIsRejected(t *testing.T) {
	first, dir := initBoardDir(t)
	issue := issueIn(t, first, model.StatusOpen)
	if _, err := first.Transition(ActionClaim, issue.ID, TransitionOptions{}); err != nil {
		t.Fatal(err)
	}

	second, err := Open(dir, session.Identity{Agent: "agent-b", Session: "sess-2"})
	if err != nil {
		t.Fatal(err)
	}
	other := issueIn(t, first, model.StatusOpen)
	_, err = second.Transition(ActionClaim, other.ID, TransitionOptions{})
	var busy *session.WorktreeBusyError
	if !errors.As(err, &busy) {
		t.Fatalf("두 번째 에이전트의 claim = %v, WorktreeBusyError 여야 합니다", err)
	}
}

// T4.9 — --allow-shared-worktree 는 그 거부를 우회한다.
//
// 컨테이너처럼 작업 트리 충돌이 구조적으로 없는 환경을 위한 탈출구다.
func TestAllowSharedWorktreeBypassesRejection(t *testing.T) {
	first, dir := initBoardDir(t)
	claimed := issueIn(t, first, model.StatusClaimed)
	_ = claimed

	second, err := Open(dir, session.Identity{Agent: "agent-b", Session: "sess-2"})
	if err != nil {
		t.Fatal(err)
	}
	second.allowSharedWorktree = true

	target := issueIn(t, first, model.StatusOpen)
	got, err := second.Transition(ActionClaim, target.ID, TransitionOptions{})
	if err != nil {
		t.Fatalf("--allow-shared-worktree 인데 거부됐다: %v", err)
	}
	if got.Owner != "agent-b" {
		t.Fatalf("owner = %q, want agent-b", got.Owner)
	}
}
