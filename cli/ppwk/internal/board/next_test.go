package board

import (
	"fmt"

	"math/rand/v2"
	"testing"
	"time"

	"github.com/go-git/go-git/v6/plumbing"

	"github.com/ironpark/toolz/cli/ppwk/internal/faultstore"
	"github.com/ironpark/toolz/cli/ppwk/internal/gitobj"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
	"github.com/ironpark/toolz/cli/ppwk/internal/session"
)

// ids 는 후보 목록을 비교하기 쉬운 형태로 바꾼다.
func ids(issues []*Issue) []string {
	out := make([]string, 0, len(issues))
	for _, i := range issues {
		out = append(out, i.ID)
	}
	return out
}

func mustAdd(t *testing.T, b *Board, opts AddOptions) *Issue {
	t.Helper()
	if opts.Title == "" {
		opts.Title = "제목"
	}
	issue, err := b.Add(opts)
	if err != nil {
		t.Fatalf("Add(%+v) = %v", opts, err)
	}
	return issue
}

func mustNext(t *testing.T, b *Board, opts NextOptions) *NextResult {
	t.Helper()
	result, err := b.Next(opts)
	if err != nil {
		t.Fatalf("Next(%+v) = %v", opts, err)
	}
	return result
}

// T5.1 후보가 없으면 빈 결과다. 오류가 아니다.
func TestNextWithNoCandidates(t *testing.T) {
	b := initBoard(t)
	result := mustNext(t, b, NextOptions{Claim: true})
	if len(result.Candidates) != 0 || result.Claimed != nil {
		t.Fatalf("후보=%v claimed=%v", ids(result.Candidates), result.Claimed)
	}

	// claim 된 이슈도 후보가 아니다 — open 만 후보다.
	issueIn(t, b, model.StatusClaimed)
	result = mustNext(t, b, NextOptions{Claim: true})
	if len(result.Candidates) != 0 {
		t.Fatalf("후보=%v", ids(result.Candidates))
	}
}

// T5.2 정렬은 plan priority → seq → priority → created_at 순이다.
func TestNextSortOrder(t *testing.T) {
	b := initBoard(t)

	// 같은 plan 안에서는 seq 가 priority 를 이긴다. high 인 task 가 계획
	// 순서를 뛰어넘으면 안 된다 (§7.2).
	first := mustAdd(t, b, AddOptions{Title: "seq 10 low", Plan: "P01", Phase: "p1", Seq: 10,
		Priority: model.PriorityLow})
	second := mustAdd(t, b, AddOptions{Title: "seq 20 high", Plan: "P01", Phase: "p1", Seq: 20,
		Priority: model.PriorityHigh})
	// 같은 seq 구간에서는 priority 가 정한다.
	lowNoPlan := mustAdd(t, b, AddOptions{Title: "plan 없음 low", Priority: model.PriorityLow})
	highNoPlan := mustAdd(t, b, AddOptions{Title: "plan 없음 high", Priority: model.PriorityHigh})

	got := ids(mustNext(t, b, NextOptions{}).Candidates)
	want := []string{highNoPlan.ID, lowNoPlan.ID, first.ID, second.ID}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("순서 = %v, want %v", got, want)
	}
}

// T5.2b plan priority 가 가장 앞선 키다.
func TestPlanPriorityLeadsSort(t *testing.T) {
	b := initBoard(t)
	high := mustAdd(t, b, AddOptions{Title: "높은 plan", Plan: "P01", Phase: "p1", Seq: 90})
	low := mustAdd(t, b, AddOptions{Title: "낮은 plan", Plan: "P02", Phase: "p1", Seq: 10})
	writePlan(t, b, model.Plan{Schema: 1, ID: "P01", Title: "P01", Status: model.PlanActive,
		Priority: model.PriorityHigh})
	writePlan(t, b, model.Plan{Schema: 1, ID: "P02", Title: "P02", Status: model.PlanActive,
		Priority: model.PriorityLow})

	got := ids(mustNext(t, b, NextOptions{}).Candidates)
	if fmt.Sprint(got) != fmt.Sprint([]string{high.ID, low.ID}) {
		t.Fatalf("순서 = %v, want [%s %s]", got, high.ID, low.ID)
	}
}

// T5.3 depends_on 이 충족되지 않은 이슈는 후보가 아니다.
func TestDependenciesGateCandidates(t *testing.T) {
	b := initBoard(t)
	blocker := mustAdd(t, b, AddOptions{Title: "선행"})
	waiter := mustAdd(t, b, AddOptions{Title: "후속", DependsOn: []string{blocker.ID}})
	missing := mustAdd(t, b, AddOptions{Title: "없는 것에 의존", DependsOn: []string{"T999"}})

	got := ids(mustNext(t, b, NextOptions{}).Candidates)
	if fmt.Sprint(got) != fmt.Sprint([]string{blocker.ID}) {
		t.Fatalf("후보 = %v, want [%s]", got, blocker.ID)
	}
	if _, err := b.Show(missing.ID); err != nil {
		t.Fatal(err)
	}

	// 선행이 done 이 되면 후속이 등장한다.
	transitionAll(t, b, blocker.ID, ActionStart, ActionDone)
	got = ids(mustNext(t, b, NextOptions{}).Candidates)
	if fmt.Sprint(got) != fmt.Sprint([]string{waiter.ID}) {
		t.Fatalf("후보 = %v, want [%s]", got, waiter.ID)
	}
}

// T5.3a cancelled 는 의존을 충족하지 않는다. 취소는 일이 끝난 것이 아니다.
func TestCancelledDependencyDoesNotSatisfy(t *testing.T) {
	b := initBoard(t)
	blocker := mustAdd(t, b, AddOptions{Title: "선행"})
	mustAdd(t, b, AddOptions{Title: "후속", DependsOn: []string{blocker.ID}})
	transitionAll(t, b, blocker.ID, ActionCancel)

	if got := ids(mustNext(t, b, NextOptions{}).Candidates); len(got) != 0 {
		t.Fatalf("후보 = %v, want 없음", got)
	}
}

// T5.3b priority none 은 후보에서 빠지되 상태는 open 그대로다.
func TestBacklogIsNotACandidate(t *testing.T) {
	b := initBoard(t)
	backlog := mustAdd(t, b, AddOptions{Title: "언젠가", Priority: model.PriorityNone})

	if got := ids(mustNext(t, b, NextOptions{}).Candidates); len(got) != 0 {
		t.Fatalf("후보 = %v, want 없음", got)
	}
	after, err := b.Show(backlog.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != model.StatusOpen {
		t.Fatalf("상태 = %s, want open", after.Status)
	}
}

// T5.3c priority 를 올리면 후보에 등장한다.
func TestBacklogBecomesCandidateAfterEdit(t *testing.T) {
	b := initBoard(t)
	backlog := mustAdd(t, b, AddOptions{Title: "언젠가", Priority: model.PriorityNone})

	if _, err := b.Mutate(Mutation{ID: backlog.ID, Event: "edit", Apply: func(i *model.Issue) error {
		i.Priority = model.PriorityLow
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustNext(t, b, NextOptions{}).Candidates); fmt.Sprint(got) != fmt.Sprint([]string{backlog.ID}) {
		t.Fatalf("후보 = %v, want [%s]", got, backlog.ID)
	}
}

// T5.4 의존 대상이 archive 에 있어도 done 으로 인식한다.
//
// issues/ 만 보면 실패한다 — 완료된 이슈는 archive 로 옮겨지므로 후속 작업이
// 영원히 후보에서 빠지는 형태로 나타난다.
func TestDependencyFoundInArchive(t *testing.T) {
	b := initBoard(t)
	blocker := mustAdd(t, b, AddOptions{Title: "선행"})
	waiter := mustAdd(t, b, AddOptions{Title: "후속", DependsOn: []string{blocker.ID}})
	// done 이 이동을 겸한다 (단계 6). 의존 검사가 archive 를 안 보면 여기서
	// 후속이 사라진다.
	transitionAll(t, b, blocker.ID, ActionStart, ActionDone)
	if archived, err := b.Show(blocker.ID); err != nil || !archived.Archived() {
		t.Fatalf("선행이 archive 로 가지 않았습니다: %v %v", archived, err)
	}

	got := ids(mustNext(t, b, NextOptions{}).Candidates)
	if fmt.Sprint(got) != fmt.Sprint([]string{waiter.ID}) {
		t.Fatalf("후보 = %v, want [%s]", got, waiter.ID)
	}
}

// T5.6 후보보다 에이전트가 많으면 후보 수만큼만 배정된다.
func TestMoreAgentsThanCandidates(t *testing.T) {
	b := initBoard(t)
	candidates := 2
	for range candidates {
		mustAdd(t, b, AddOptions{Title: "일감"})
	}

	claimed := 0
	for i := range 5 {
		agent := b.asAgent(fmt.Sprintf("agent-%d", i))
		result := mustNext(t, agent, NextOptions{Claim: true})
		if result.Claimed != nil {
			claimed++
		}
	}
	if claimed != candidates {
		t.Fatalf("배정 %d건, want %d", claimed, candidates)
	}
}

// T5.7 CAS 에 밀리면 같은 이슈를 재시도하지 않고 다음 후보로 넘어간다.
func TestCASFailureMovesToNextCandidate(t *testing.T) {
	b := initBoard(t)
	first := mustAdd(t, b, AddOptions{Title: "먼저", Priority: model.PriorityHigh})
	second := mustAdd(t, b, AddOptions{Title: "다음", Priority: model.PriorityLow})

	// 첫 후보의 CAS 직전에 다른 에이전트가 가져가게 한다. 4단계와 5단계
	// 사이를 정확히 노리는 것은 결함 주입으로만 결정적으로 재현된다 (D2.1).
	thief := b.asAgent("agent-thief")
	stolen := false
	racer := b.WithStore(faultstore.New(b.Store(), faultstore.Config{
		Hook: func(ref string, _, _ plumbing.Hash) {
			if stolen || ref != refstore.Issues+first.ID {
				return
			}
			stolen = true
			if _, err := thief.Transition(ActionClaim, first.ID, TransitionOptions{}); err != nil {
				t.Errorf("가로채기 실패: %v", err)
			}
		},
	}))
	// 가로채는 쪽이 worktree 임차를 먼저 쥐게 되므로, 이 테스트에서는 배타를
	// 끈다. 여기서 보려는 것은 CAS 경쟁이지 worktree 규칙이 아니다.
	racer.allowSharedWorktree = true

	result := mustNext(t, racer, NextOptions{Claim: true, MaxAttempts: 5})
	if !stolen {
		t.Fatal("가로채기가 일어나지 않았습니다")
	}
	if result.Claimed == nil || result.Claimed.ID != second.ID {
		t.Fatalf("claimed = %v, want %s", result.Claimed, second.ID)
	}
	// 같은 이슈를 다시 노렸다면 시도 횟수가 2를 넘는다.
	if result.Attempts != 2 {
		t.Fatalf("시도 %d회, want 2", result.Attempts)
	}
}

// max-attempts 는 후보가 아무리 많아도 시도를 상한에서 멈춘다.
func TestMaxAttemptsCapsClaims(t *testing.T) {
	b := initBoard(t)
	for range 5 {
		mustAdd(t, b, AddOptions{Title: "일감"})
	}
	// 전부 실패하도록 CAS 를 계속 lock busy 로 만든다.
	stuck := b.WithStore(faultstore.New(b.Store(), faultstore.Config{Seed: 1, LockBusyRate: 1}))
	result := mustNext(t, stuck, NextOptions{Claim: true, MaxAttempts: 2})
	if result.Claimed != nil {
		t.Fatalf("claimed = %v, want nil", result.Claimed)
	}
	if result.Attempts != 2 {
		t.Fatalf("시도 %d회, want 2", result.Attempts)
	}
}

// --dry-run 은 저장소를 전혀 변형하지 않는다 — reap 도 하지 않는다.
func TestNextDryRunDoesNotReap(t *testing.T) {
	b := initBoard(t)
	dead := issueIn(t, b, model.StatusClaimed)
	b.leases.Now = func() time.Time { return time.Now().Add(9 * time.Hour) }

	result := mustNext(t, b, NextOptions{DryRun: true, Claim: true})
	if result.Claimed != nil {
		t.Fatalf("dry-run 이 claim 했습니다: %v", result.Claimed)
	}
	after, err := b.Show(dead.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != model.StatusClaimed || after.Commit != dead.Commit {
		t.Fatalf("dry-run 이 저장소를 바꿨습니다: %s %s", after.Status, after.Commit)
	}
}

// next 는 회수를 겸한다. 죽은 소유자의 이슈가 곧바로 후보가 되어야 한다 (§7.2 1단계).
func TestNextReapsBeforeSelecting(t *testing.T) {
	b := initBoard(t)
	dead := issueIn(t, b, model.StatusClaimed)
	b.leases.Now = func() time.Time { return time.Now().Add(9 * time.Hour) }

	agent := b.asAgent("agent-b")
	agent.leases.Now = b.leases.Now
	result := mustNext(t, agent, NextOptions{Claim: true})
	if result.Claimed == nil || result.Claimed.ID != dead.ID {
		t.Fatalf("claimed = %v, want %s", result.Claimed, dead.ID)
	}
}

// --label 은 capability 필터다.
func TestLabelFilter(t *testing.T) {
	b := initBoard(t)
	mustAdd(t, b, AddOptions{Title: "라벨 없음"})
	tagged := mustAdd(t, b, AddOptions{Title: "go", Labels: []string{"go", "cli"}})

	got := ids(mustNext(t, b, NextOptions{Label: "go"}).Candidates)
	if fmt.Sprint(got) != fmt.Sprint([]string{tagged.ID}) {
		t.Fatalf("후보 = %v, want [%s]", got, tagged.ID)
	}
}

// 순환 의존은 후보를 영원히 만들지 않되, 무한 루프에 빠지지 않는다.
func TestCycleTerminates(t *testing.T) {
	b := initBoard(t)
	a := mustAdd(t, b, AddOptions{Title: "a"})
	c := mustAdd(t, b, AddOptions{Title: "b", DependsOn: []string{a.ID}})
	if _, err := b.Mutate(Mutation{ID: a.ID, Event: "edit", Apply: func(i *model.Issue) error {
		i.DependsOn = []string{c.ID}
		return nil
	}}); err != nil {
		t.Fatal(err)
	}

	done := make(chan []string, 1)
	go func() { done <- ids(mustNext(t, b, NextOptions{}).Candidates) }()
	select {
	case got := <-done:
		if len(got) != 0 {
			t.Fatalf("후보 = %v, want 없음", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("순환 의존에서 멈추지 않았습니다")
	}
}

// asAgent 는 같은 저장소를 다른 신원으로 보는 사본이다.
//
// worktree 배타는 켜지 않는다 — 한 프로세스 안에서 여러 신원을 흉내 내는
// 것이므로 그 규칙이 검증 대상이 아니다 (T4.10 이 따로 본다).
func (b *Board) asAgent(agent string) *Board {
	clone := *b
	clone.identity = session.Identity{Agent: agent, Session: agent + "-sess"}
	clone.leases = session.NewRegistry("", "", clone.identity)
	clone.leases.Dir, clone.leases.Worktree = b.leases.Dir, b.leases.Worktree
	clone.leaseSnapshot = clone.leases.List
	clone.allowSharedWorktree = true
	return &clone
}

// transitionAll 은 전이를 차례로 수행한다.
func transitionAll(t *testing.T, b *Board, id string, actions ...Action) {
	t.Helper()
	for _, a := range actions {
		if _, err := b.Transition(a, id, TransitionOptions{}); err != nil {
			t.Fatalf("%s %s = %v", a, id, err)
		}
	}
}

// writePlan 은 plan 문서를 직접 써넣는다. plan 명령은 아직 없다.
func writePlan(t *testing.T, b *Board, plan model.Plan) {
	t.Helper()
	plan.CreatedAt, plan.UpdatedAt = model.Now(), model.Now()
	hash, err := gitobj.Write(b.repo, gitobj.Commit{
		Doc:     plan,
		DocName: gitobj.FilePlan,
		Subject: "plan-new: " + plan.Title,
		Author:  b.signature(plan.CreatedAt),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Store().CAS(refstore.Plans+plan.ID, hash, plumbing.ZeroHash); err != nil {
		t.Fatal(err)
	}
}

// ---- fuzz ----

// F5.1 후보 선정 불변식.
//
// 임의 바이트가 아니라 seed 로 보드를 결정적으로 생성한다. 무작위 바이트를
// 넣으면 파서만 흔들고 규칙은 한 번도 밟지 않는다.
func FuzzCandidates(f *testing.F) {
	f.Add(uint64(1))
	f.Add(uint64(20260903))
	f.Fuzz(func(t *testing.T, seed uint64) {
		issues, lookup := generateBoard(seed)
		sel := selector{
			status:       lookup,
			planPriority: func(string) model.Priority { return model.PriorityMed },
		}
		got := sel.pick(issues)
		for _, c := range got {
			if c.Status != model.StatusOpen {
				t.Fatalf("%s 의 상태가 %s 입니다", c.ID, c.Status)
			}
			if c.Priority == model.PriorityNone {
				t.Fatalf("%s 가 백로그인데 후보입니다", c.ID)
			}
			for _, dep := range c.DependsOn {
				status, ok := lookup(dep)
				if !ok || status != model.StatusDone {
					t.Fatalf("%s 의 의존 %s 가 %q(있음=%v) 입니다", c.ID, dep, status, ok)
				}
			}
		}
	})
}

// F5.2 비교 함수가 전순서인가.
//
// 비일관 비교자는 정렬 결과가 실행마다 달라지는 형태로 나타나 재현이 매우
// 어렵다. 반사성·반대칭성·추이성을 무작위 3원소로 직접 확인한다.
func FuzzSortOrder(f *testing.F) {
	f.Add(uint64(7))
	f.Add(uint64(99))
	f.Fuzz(func(t *testing.T, seed uint64) {
		issues, _ := generateBoard(seed)
		if len(issues) < 3 {
			return
		}
		planPriority := planPriorityBySuffix
		sel := selector{
			status:       func(string) (model.Status, bool) { return model.StatusDone, true },
			planPriority: planPriority,
		}
		rng := rand.New(rand.NewPCG(seed, ^seed))
		for range 200 {
			a := &issues[rng.IntN(len(issues))].Issue
			b := &issues[rng.IntN(len(issues))].Issue
			c := &issues[rng.IntN(len(issues))].Issue

			if sel.compare(a, a) != 0 {
				t.Fatalf("반사성 위반: %s", a.ID)
			}
			ab, ba := sel.compare(a, b), sel.compare(b, a)
			if sign(ab) != -sign(ba) {
				t.Fatalf("반대칭성 위반: %s vs %s (%d, %d)", a.ID, b.ID, ab, ba)
			}
			if sign(ab) <= 0 && sign(sel.compare(b, c)) <= 0 && sign(sel.compare(a, c)) > 0 {
				t.Fatalf("추이성 위반: %s ≤ %s ≤ %s", a.ID, b.ID, c.ID)
			}
		}
	})
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

// planPriorityBySuffix 는 plan ID 마지막 글자로 우선순위를 정한다. 저장소 없이
// plan priority 축을 흔들기 위한 것이다.
func planPriorityBySuffix(id string) model.Priority {
	if id == "" {
		return model.PriorityMed
	}
	switch id[len(id)-1] % 3 {
	case 0:
		return model.PriorityHigh
	case 1:
		return model.PriorityMed
	}
	return model.PriorityLow
}

// generateBoard 는 seed 로 이슈 집합을 결정적으로 만든다. 순환 의존도 나온다.
func generateBoard(seed uint64) ([]*Issue, func(string) (model.Status, bool)) {
	rng := rand.New(rand.NewPCG(seed, seed^0x5deece66d))
	statuses := []model.Status{model.StatusOpen, model.StatusClaimed, model.StatusWorking,
		model.StatusBlocked, model.StatusDone, model.StatusCancelled}
	priorities := []model.Priority{model.PriorityHigh, model.PriorityMed, model.PriorityLow, model.PriorityNone}
	plans := []string{"", "P01", "P02", "P03"}

	n := 1 + rng.IntN(24)
	all := make([]*Issue, 0, n)
	byID := make(map[string]*Issue, n)
	base := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	for i := range n {
		issue := &Issue{Issue: model.Issue{
			Schema:    model.SchemaVersion,
			ID:        formatIssueID(i + 1),
			Title:     "t",
			Status:    statuses[rng.IntN(len(statuses))],
			Priority:  priorities[rng.IntN(len(priorities))],
			Plan:      plans[rng.IntN(len(plans))],
			CreatedAt: model.NewTimestamp(base.Add(time.Duration(rng.IntN(5)) * time.Second)),
		}}
		if issue.Plan != "" {
			issue.Phase = "p1"
			issue.Seq = 10 * rng.IntN(4)
		}
		all = append(all, issue)
		byID[issue.ID] = issue
	}
	// 의존을 나중에 건다. 앞뒤를 가리지 않으므로 순환이 자연스럽게 생긴다.
	for _, issue := range all {
		for range rng.IntN(3) {
			dep := formatIssueID(1 + rng.IntN(n+2)) // n+2 로 없는 ID 도 섞는다
			if dep != issue.ID {
				issue.DependsOn = append(issue.DependsOn, dep)
			}
		}
	}
	return all, func(id string) (model.Status, bool) {
		issue, ok := byID[id]
		if !ok {
			return "", false
		}
		return issue.Status, true
	}
}
