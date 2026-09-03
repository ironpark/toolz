package board

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/ironpark/toolz/cli/ppwk/internal/gitobj"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

// writeIssueRaw 는 issue.json 과 trailer 를 임의로 써넣는다.
//
// 정상 경로로는 만들 수 없는 손상이 필요하다. fsck 는 바로 그런 상태를 위해
// 있는 도구이므로, 테스트도 정상 경로 밖에서 상태를 만들어야 한다.
func writeIssueRaw(t *testing.T, b *Board, id string, issue model.Issue, trailers []gitobj.Trailer) {
	t.Helper()
	ref := refstore.Issues + id
	old, err := b.Store().Get(ref)
	if err != nil {
		old = plumbing.ZeroHash
	}
	hash, err := gitobj.Write(b.repo, gitobj.Commit{
		Doc: issue, DocName: gitobj.FileIssue,
		Subject: "fsck-test: " + issue.Title, Trailers: trailers,
		Author: b.signature(model.Now()), Parent: old,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Store().CAS(ref, hash, old); err != nil {
		t.Fatal(err)
	}
}

// findingsFor 는 특정 검사 항목의 결과만 추린다. 다른 항목이 함께 걸리는 것은
// 정상이므로, 보려는 항목만 본다.
func findingsFor(t *testing.T, b *Board, check string) []Finding {
	t.Helper()
	all, err := b.Fsck(FsckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var got []Finding
	for _, f := range all {
		if f.Check == check {
			got = append(got, f)
		}
	}
	return got
}

// expectFinding 은 항목이 정확히 한 번, 주어진 이슈에 대해 걸리는지 본다.
func expectFinding(t *testing.T, b *Board, check, id, level string) Finding {
	t.Helper()
	got := findingsFor(t, b, check)
	if len(got) != 1 {
		t.Fatalf("%s 발견 %d건: %v", check, len(got), got)
	}
	if got[0].ID != id || got[0].Level != level {
		t.Fatalf("%s = %+v, want id=%s level=%s", check, got[0], id, level)
	}
	return got[0]
}

// T8.6 정상 저장소에서는 아무것도 걸리지 않는다.
func TestFsckCleanBoard(t *testing.T) {
	b := initBoard(t)
	first := mustAdd(t, b, AddOptions{Title: "하나"})
	second := mustAdd(t, b, AddOptions{Title: "둘", DependsOn: []string{first.ID}})
	transitionAll(t, b, first.ID, ActionStart, ActionDone)
	transitionAll(t, b, second.ID, ActionClaim)

	findings, err := b.Fsck(FsckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("정상 보드에서 %d건: %v", len(findings), findings)
	}
	if HasErrors(findings) {
		t.Fatal("HasErrors 가 참입니다")
	}
}

// T8.3 §9.3 의 각 항목이 실제로 검출되는지 — 항목당 하나씩.

func TestFsckDetectsTrailerMismatch(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})

	// issue.json 은 working 인데 trailer 는 open 이라고 말한다.
	issue.Status = model.StatusWorking
	writeIssueRaw(t, b, issue.ID, issue.Issue, []gitobj.Trailer{
		{Key: gitobj.KeyTitle, Value: issue.Title},
		{Key: gitobj.KeyStatus, Value: string(model.StatusOpen)},
		{Key: gitobj.KeyPriority, Value: string(issue.Priority)},
	})
	expectFinding(t, b, CheckTrailerStatus, issue.ID, LevelError)
}

func TestFsckDetectsMissingDependency(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상", DependsOn: []string{"T999"}})
	expectFinding(t, b, CheckMissingDep, issue.ID, LevelError)
}

func TestFsckDetectsDependencyCycle(t *testing.T) {
	b := initBoard(t)
	a := mustAdd(t, b, AddOptions{Title: "a"})
	c := mustAdd(t, b, AddOptions{Title: "b", DependsOn: []string{a.ID}})
	if _, err := b.Mutate(Mutation{ID: a.ID, Event: "edit", Apply: func(i *model.Issue) error {
		i.DependsOn = []string{c.ID}
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if got := findingsFor(t, b, CheckDependencyCycle); len(got) == 0 {
		t.Fatal("순환을 찾지 못했습니다")
	}
}

func TestFsckDetectsOwnerWithoutLease(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})
	transitionAll(t, b, issue.ID, ActionClaim)

	// 잠금 디렉터리를 통째로 없앤다. 기계를 갈아엎었거나 누가 지운 상황이다.
	if err := os.RemoveAll(b.leases.Dir); err != nil {
		t.Fatal(err)
	}
	expectFinding(t, b, CheckOwnerNoLease, issue.ID, LevelError)
}

func TestFsckDetectsTerminalNotArchived(t *testing.T) {
	b := initBoard(t)
	issue := doneInIssues(t, b)
	expectFinding(t, b, CheckTerminalInPlace, issue.ID, LevelError)
}

func TestFsckDetectsSchemaMismatch(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})

	future := issue.Issue
	future.Schema = model.SchemaVersion + 1
	writeIssueRaw(t, b, issue.ID, future, issueTrailers(future, "s"))
	expectFinding(t, b, CheckSchemaVersion, issue.ID, LevelError)
}

func TestFsckDetectsMissingPlan(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상", Plan: "P99", Phase: "p1", Seq: 10})
	expectFinding(t, b, CheckMissingPlan, issue.ID, LevelError)
}

func TestFsckDetectsMissingPhase(t *testing.T) {
	b := initBoard(t)
	writePlan(t, b, model.Plan{Schema: 1, ID: "P01", Title: "계획", Status: model.PlanActive,
		Priority: model.PriorityMed, Phases: []model.Phase{{ID: "p1", Title: "하나", Gate: model.GateAllDone}}})
	issue := mustAdd(t, b, AddOptions{Title: "대상", Plan: "P01", Phase: "p9", Seq: 10})
	expectFinding(t, b, CheckMissingPhase, issue.ID, LevelError)
}

func TestFsckDetectsPartialPlanFields(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})

	// Validate 가 막는 조합이므로 정상 경로로는 만들 수 없다.
	broken := issue.Issue
	broken.Plan = "P01"
	writeIssueRaw(t, b, issue.ID, broken, []gitobj.Trailer{
		{Key: gitobj.KeyTitle, Value: broken.Title},
		{Key: gitobj.KeyStatus, Value: string(broken.Status)},
		{Key: gitobj.KeyPriority, Value: string(broken.Priority)},
	})
	expectFinding(t, b, CheckPartialPlan, issue.ID, LevelError)
}

func TestFsckDetectsClosedPlanWithOpenTask(t *testing.T) {
	b := initBoard(t)
	writePlan(t, b, model.Plan{Schema: 1, ID: "P01", Title: "계획", Status: model.PlanClosed,
		Priority: model.PriorityMed, Phases: []model.Phase{{ID: "p1", Title: "하나", Gate: model.GateAllDone}}})
	mustAdd(t, b, AddOptions{Title: "미완", Plan: "P01", Phase: "p1", Seq: 10})
	expectFinding(t, b, CheckClosedPlanOpen, "P01", LevelError)
}

func TestFsckDetectsEmptyPhase(t *testing.T) {
	b := initBoard(t)
	writePlan(t, b, model.Plan{Schema: 1, ID: "P01", Title: "계획", Status: model.PlanActive,
		Priority: model.PriorityMed, Phases: []model.Phase{{ID: "p1", Title: "빈 단계", Gate: model.GateAllDone}}})
	expectFinding(t, b, CheckEmptyPhase, "P01", LevelWarn)
}

func TestFsckDetectsDuplicateSeq(t *testing.T) {
	b := initBoard(t)
	writePlan(t, b, model.Plan{Schema: 1, ID: "P01", Title: "계획", Status: model.PlanActive,
		Priority: model.PriorityMed, Phases: []model.Phase{{ID: "p1", Title: "하나", Gate: model.GateAllDone}}})
	mustAdd(t, b, AddOptions{Title: "첫째", Plan: "P01", Phase: "p1", Seq: 10})
	second := mustAdd(t, b, AddOptions{Title: "둘째", Plan: "P01", Phase: "p1", Seq: 10})
	expectFinding(t, b, CheckDuplicateSeq, second.ID, LevelWarn)
}

func TestFsckDetectsUnknownAdvancedPhase(t *testing.T) {
	b := initBoard(t)
	writePlan(t, b, model.Plan{Schema: 1, ID: "P01", Title: "계획", Status: model.PlanActive,
		Priority: model.PriorityMed,
		Phases:   []model.Phase{{ID: "p1", Title: "하나", Gate: model.GateManual}},
		// Validate 가 막는 조합이지만, 손상된 문서는 실재할 수 있다.
		AdvancedPhases: []string{"p9"}})
	expectFinding(t, b, CheckAdvancedPhase, "P01", LevelError)
}

func TestFsckWarnsOnCancelledDependency(t *testing.T) {
	b := initBoard(t)
	blocker := mustAdd(t, b, AddOptions{Title: "선행"})
	waiter := mustAdd(t, b, AddOptions{Title: "후속", DependsOn: []string{blocker.ID}})
	transitionAll(t, b, blocker.ID, ActionCancel)
	expectFinding(t, b, CheckCancelledDep, waiter.ID, LevelWarn)
}

func TestFsckWarnsOnBacklogDependency(t *testing.T) {
	b := initBoard(t)
	backlog := mustAdd(t, b, AddOptions{Title: "언젠가", Priority: model.PriorityNone})
	waiter := mustAdd(t, b, AddOptions{Title: "후속", DependsOn: []string{backlog.ID}})
	expectFinding(t, b, CheckBacklogDep, waiter.ID, LevelWarn)
}

func TestFsckWarnsOnStaleLock(t *testing.T) {
	b, dir := initBoardDir(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})

	// 내용은 진짜 갱신 중인 .lock 과 같은 모양이어야 한다. 빈 파일이면
	// 이 테스트가 검사 대상이 아니라 파서를 시험하게 된다.
	hash, err := b.Store().Get(refstore.Issues + issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(dir, ".git", "refs", "ppwk", "issues", issue.ID+".lock")
	if err := os.WriteFile(lock, []byte(hash.String()+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := findingsFor(t, b, CheckStaleLock)
	if len(got) != 1 || got[0].Level != LevelWarn {
		t.Fatalf("발견 = %v", got)
	}
	if !strings.Contains(got[0].Message, lock) {
		t.Fatalf("경로가 없습니다: %q", got[0].Message)
	}
	// 자동으로 지우지 않는다. 진짜로 다른 프로세스가 쓰는 중일 수 있다.
	if _, err := b.Fsck(FsckOptions{Fix: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("--fix 가 .lock 을 지웠습니다: %v", err)
	}
}

// T8.4 --fix 가 trailer 불일치를 복구한다.
func TestFsckFixRepairsTrailer(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})
	broken := issue.Issue
	broken.Status = model.StatusWorking
	broken.Owner, broken.Session = b.identity.Agent, b.identity.Session
	writeIssueRaw(t, b, issue.ID, broken, []gitobj.Trailer{
		{Key: gitobj.KeyTitle, Value: broken.Title},
		{Key: gitobj.KeyStatus, Value: string(model.StatusOpen)},
		{Key: gitobj.KeyPriority, Value: string(broken.Priority)},
	})

	findings, err := b.Fsck(FsckOptions{Fix: true})
	if err != nil {
		t.Fatal(err)
	}
	fixed := false
	for _, f := range findings {
		if f.Check == CheckTrailerStatus {
			if !f.Fixed {
				t.Fatalf("고치지 못했습니다: %+v", f)
			}
			fixed = true
		}
	}
	if !fixed {
		t.Fatalf("trailer 불일치를 찾지 못했습니다: %v", findings)
	}
	if got := findingsFor(t, b, CheckTrailerStatus); len(got) != 0 {
		t.Fatalf("고친 뒤에도 남았습니다: %v", got)
	}
	// 목록이 issue.json 과 같은 것을 말한다.
	entries, err := b.List(ListOptions{Status: []model.Status{model.StatusWorking}})
	if err != nil || len(entries) != 1 {
		t.Fatalf("list = %v, %v", entries, err)
	}
}

// --fix 는 archive 이동도 처리한다.
func TestFsckFixMovesToArchive(t *testing.T) {
	b := initBoard(t)
	issue := doneInIssues(t, b)

	if _, err := b.Fsck(FsckOptions{Fix: true}); err != nil {
		t.Fatal(err)
	}
	if refExists(t, b, refstore.Issues+issue.ID) {
		t.Fatal("issues/ 에 남아 있습니다")
	}
	if !refExists(t, b, refstore.Archive+issue.ID) {
		t.Fatal("archive/ 로 가지 않았습니다")
	}
}

// T8.5 --fix 는 데이터를 잃지 않는다.
func TestFsckFixPreservesData(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{
		Title: "대상", Priority: model.PriorityHigh, Labels: []string{"go", "cli"},
		Body: []byte("본문 여러 줄\n둘째 줄\n"),
	})
	broken := issue.Issue
	broken.Status = model.StatusBlocked
	writeIssueRaw(t, b, issue.ID, broken, []gitobj.Trailer{
		{Key: gitobj.KeyStatus, Value: string(model.StatusOpen)},
		{Key: gitobj.KeyPriority, Value: string(broken.Priority)},
	})
	before, err := b.Show(issue.ID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := b.Fsck(FsckOptions{Fix: true}); err != nil {
		t.Fatal(err)
	}

	after, err := b.Show(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Title != before.Title || after.Status != before.Status ||
		after.Priority != before.Priority || string(after.Body) != string(before.Body) ||
		strings.Join(after.Labels, ",") != strings.Join(before.Labels, ",") ||
		!after.CreatedAt.Time.Equal(before.CreatedAt.Time) {
		t.Fatalf("--fix 가 내용을 바꿨습니다:\nbefore=%+v\nafter=%+v", before.Issue, after.Issue)
	}
	// 이력도 남는다 — 고친 사실 자체가 이력이다.
	events, err := b.History(issue.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 3 || !strings.HasPrefix(events[0].Subject, "fsck") {
		t.Fatalf("events = %v", events)
	}
}

// 손상된 이슈를 만나도 나머지 검사는 계속된다.
func TestFsckContinuesPastUnreadableIssue(t *testing.T) {
	b := initBoard(t)
	broken := mustAdd(t, b, AddOptions{Title: "깨진 것"})
	other := mustAdd(t, b, AddOptions{Title: "정상", DependsOn: []string{"T999"}})

	// issue.json 이 없는 tree 를 만들어 읽기를 실패시킨다.
	old, err := b.Store().Get(refstore.Issues + broken.ID)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := gitobj.Write(b.repo, gitobj.Commit{
		Doc: map[string]string{"note": "이것은 이슈가 아닙니다"}, DocName: "other.json",
		Subject: "broken", Author: b.signature(model.Now()), Parent: old,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Store().CAS(refstore.Issues+broken.ID, hash, old); err != nil {
		t.Fatal(err)
	}

	expectFinding(t, b, CheckUnreadable, broken.ID, LevelError)
	expectFinding(t, b, CheckMissingDep, other.ID, LevelError)
}

// T8.7 history 가 이벤트 subject 를 그대로 보여준다.
func TestHistoryShowsEventSubjects(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})
	if _, err := b.Transition(ActionStart, issue.ID, TransitionOptions{Message: "먼저 훑어봄"}); err != nil {
		t.Fatal(err)
	}
	transitionAll(t, b, issue.ID, ActionBlock)

	events, err := b.History(issue.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"block: 대상", "start: 대상 — 먼저 훑어봄", "create: 대상"}
	if len(events) != len(want) {
		t.Fatalf("이벤트 %d개: %v", len(events), events)
	}
	for i, subject := range want {
		if events[i].Subject != subject {
			t.Fatalf("events[%d].Subject = %q, want %q", i, events[i].Subject, subject)
		}
		if events[i].Who != b.identity.Agent || events[i].Short == "" {
			t.Fatalf("events[%d] = %+v", i, events[i])
		}
	}

	// -n 은 개수를 자른다. 이력이 매우 길 때를 위한 것이다.
	short, err := b.History(issue.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(short) != 1 || short[0].Subject != want[0] {
		t.Fatalf("-n 1 = %v", short)
	}
}

// T9.x — phase 가 없는 plan 은 경고다.
//
// 오류가 아니다. 방금 만든 plan 은 정상적으로 이 상태를 거친다. 다만 이
// 상태로 task 를 붙이면 gate 판정에서 전부 걸려 next 가 영원히 비므로,
// 조용히 두지 않는다.
func TestFsckWarnsOnPlanWithoutPhases(t *testing.T) {
	b := initBoard(t)
	plan := makePlan(t, b, "phase 없는 plan", model.PriorityMed)
	expectFinding(t, b, CheckPlanNoPhases, plan.ID, LevelWarn)
}
