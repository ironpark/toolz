package board

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/session"
)

func TestReapByStateAndSession(t *testing.T) {
	b := initBoard(t)
	claimed := issueIn(t, b, model.StatusClaimed)
	working := issueIn(t, b, model.StatusWorking)
	blocked := issueIn(t, b, model.StatusBlocked)
	b.leases.Now = func() time.Time { return time.Now().Add(9 * time.Hour) }
	reaped, err := b.Reap(ReapOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reaped) != 3 {
		t.Fatalf("reaped=%d", len(reaped))
	}
	for _, tc := range []struct {
		id     string
		status model.Status
	}{{claimed.ID, model.StatusOpen}, {working.ID, model.StatusOpen}, {blocked.ID, model.StatusBlocked}} {
		i, err := b.Show(tc.id)
		if err != nil {
			t.Fatal(err)
		}
		if i.Status != tc.status || i.Owner != "" || i.Session != "" {
			t.Fatalf("%s = %s owner=%q session=%q", tc.id, i.Status, i.Owner, i.Session)
		}
	}
}

func TestReapLiveOwnerAndNoTargetsDoNotWrite(t *testing.T) {
	b := initBoard(t)
	issue := issueIn(t, b, model.StatusClaimed)
	before := issue.Commit
	reaped, err := b.Reap(ReapOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reaped) != 0 {
		t.Fatalf("reaped=%v", reaped)
	}
	after, err := b.Show(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Commit != before {
		t.Fatal("live issue was written")
	}
}

func TestSameAgentDifferentSessionIsReaped(t *testing.T) {
	b := initBoard(t)
	old := issueIn(t, b, model.StatusClaimed)
	b.identity.Session = "new-session"
	b.leases.Identity = b.identity
	b.allowSharedWorktree = true
	if _, err := b.leases.Register(true); err != nil {
		t.Fatal(err)
	}
	reaped, err := b.Reap(ReapOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reaped) != 1 || reaped[0].ID != old.ID {
		t.Fatalf("reaped=%v", reaped)
	}
}

// T4.18 — 생존 확인 횟수가 이슈 수·소유자 수에 비례하지 않는다.
//
// 소유자마다 잠금 파일을 다시 열면 같은 명령 안에서 판정 기준이 흔들리고,
// 보드가 커질수록 I/O 가 이슈 수를 따라 늘어난다.
func TestReapReadsLeasesOnce(t *testing.T) {
	b := initBoard(t)
	for range 5 {
		issueIn(t, b, model.StatusClaimed)
	}
	calls := 0
	inner := b.leaseSnapshot
	b.leaseSnapshot = func() []model.Lease { calls++; return inner() }
	b.leases.Now = func() time.Time { return time.Now().Add(9 * time.Hour) }

	reaped, err := b.Reap(ReapOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reaped) != 5 {
		t.Fatalf("reaped=%d, want 5", len(reaped))
	}
	if calls != 1 {
		t.Fatalf("잠금 디렉터리를 %d번 읽었다, want 1", calls)
	}
}

// T4.11 — 여러 프로세스가 동시에 reap 해도 회수는 정확히 1회다.
//
// 회수는 CAS 를 거치므로 경쟁에서 밀린 쪽은 조용히 건너뛴다. 두 번 회수되면
// 나중 것이 그 사이 새로 claim 한 에이전트의 소유권을 지운다.
func TestConcurrentReapRecoversOnce(t *testing.T) {
	b := initBoard(t)
	issue := issueIn(t, b, model.StatusClaimed)
	dead := func() time.Time { return time.Now().Add(9 * time.Hour) }

	const n = 8
	results := make(chan int, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// 저마다 다른 세션이다. 같은 세션이면 경쟁 자체가 없다.
			//
			// 보드를 복사하지 않고 새로 연다. go-git 의 저장소 객체는
			// goroutine 안전이 아니라서, 하나를 공유하면 보려는 CAS 경쟁이
			// 아니라 저장소 내부 자료구조의 경쟁이 잡힌다.
			clone, err := Open(b.Root(), session.Identity{
				Agent: b.identity.Agent, Session: fmt.Sprintf("reaper-%d", i)})
			if err != nil {
				results <- -1
				return
			}
			clone.leases = sameLocks(b, clone.identity)
			clone.leases.Now = dead
			clone.leaseSnapshot = clone.leases.List
			clone.allowSharedWorktree = true
			<-start
			reaped, err := clone.Reap(ReapOptions{})
			if err != nil {
				results <- -1
				return
			}
			results <- len(reaped)
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	total := 0
	for got := range results {
		if got < 0 {
			t.Fatal("reap 이 오류로 끝났다")
		}
		total += got
	}
	if total != 1 {
		t.Fatalf("회수 %d회, want 1", total)
	}
	after, err := b.Show(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != model.StatusOpen || after.Owner != "" {
		t.Fatalf("status=%s owner=%q", after.Status, after.Owner)
	}
}

// T4.14 — 에이전트 현황을 ref 로 두지 않는다 (D13).
//
// ref 로 두면 에이전트가 살아있다는 사실이 저장소 히스토리에 커밋되고,
// 원격과 동기화되며, 죽은 에이전트의 흔적이 영구히 남는다. 잠금 파일은
// machine-local 이라 그런 일이 없다.
func TestNoAgentRefs(t *testing.T) {
	b := initBoard(t)
	issueIn(t, b, model.StatusWorking)
	if _, err := b.Reap(ReapOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(b.Agents()) == 0 {
		t.Fatal("잠금 파일에 에이전트 기록이 없다 — 전제가 깨졌다")
	}

	refs, err := b.Store().List("refs/ppwk/")
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs {
		if strings.Contains(ref.Ref, "/agents/") {
			t.Fatalf("에이전트 ref 가 생겼다: %s", ref.Ref)
		}
	}
}

// T4.16d — 조회는 last_activity 를 갱신하지 않는다.
//
// 갱신하면 아무 일도 하지 않는 watch 루프가 죽은 세션을 영원히 살려 둔다.
func TestQueriesDoNotRefreshActivity(t *testing.T) {
	b := initBoard(t)
	issue := issueIn(t, b, model.StatusClaimed)
	before := b.Agents()
	if len(before) != 1 {
		t.Fatalf("leases=%d", len(before))
	}

	b.leases.Now = func() time.Time { return time.Now().Add(time.Hour) }
	if _, err := b.List(ListOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Show(issue.ID); err != nil {
		t.Fatal(err)
	}
	b.Agents()

	after := b.Agents()
	if !after[0].LastActivity.Equal(before[0].LastActivity.Time) {
		t.Fatalf("조회가 last_activity 를 %s → %s 로 갱신했다",
			before[0].LastActivity, after[0].LastActivity)
	}
}

// T4.10 — 조회는 다른 세션이 worktree 를 쥐고 있어도 잠금 없이 동작한다.
func TestQueriesNeedNoWorktreeLock(t *testing.T) {
	b := initBoard(t)
	issue := issueIn(t, b, model.StatusClaimed)

	// 다른 세션이 worktree 를 점유한다. 상태 변경은 이제 거부되어야 한다.
	other := sameLocks(b, session.Identity{Agent: "other", Session: "other-1"})
	if _, err := other.Register(true); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Transition(ActionStart, issue.ID, TransitionOptions{}); err == nil {
		t.Fatal("점유 중인 worktree 에서 상태 변경이 통과했다")
	}

	if _, err := b.List(ListOptions{}); err != nil {
		t.Fatalf("list = %v", err)
	}
	if _, err := b.Show(issue.ID); err != nil {
		t.Fatalf("show = %v", err)
	}
	if _, err := b.History(issue.ID, 10); err != nil {
		t.Fatalf("history = %v", err)
	}
}

// sameLocks 는 보드와 같은 잠금 디렉터리·worktree 를 쓰는 다른 신원의
// Registry 를 만든다.
//
// NewRegistry 의 첫 인자는 common dir 이라 ppwk/locks 를 덧붙인다. 이미
// 만들어진 잠금 디렉터리를 그대로 쓰려면 덧붙이기 뒤의 값을 넣어야 한다.
func sameLocks(b *Board, ident session.Identity) *session.Registry {
	r := session.NewRegistry("", "", ident)
	r.Dir, r.Worktree = b.leases.Dir, b.leases.Worktree
	return r
}
