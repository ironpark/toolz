package e2e

import (
	"strings"
	"testing"
)

// E2E-1: 단일 에이전트 전체 수명주기.
//
// init → add → list → claim → start → done → archive 확인.
func TestLifecycle(t *testing.T) {
	f := newFixture(t)
	id := f.add("첫 작업")

	f.expectStatus(id, "open")
	f.MustRun("claim", id)
	f.expectStatus(id, "claimed")
	f.MustRun("start", id)
	f.expectStatus(id, "working")
	f.MustRun("done", id)

	// git 층: done 이 이동을 겸한다.
	if f.HasRef("refs/ppwk/issues/" + id) {
		t.Fatalf("done 후에도 issues/%s 가 남아 있습니다", id)
	}
	ref := "refs/ppwk/archive/" + id
	if !f.HasRef(ref) {
		t.Fatalf("%s 가 없습니다", ref)
	}

	// git 층: 이력이 chain 으로 남는다.
	subjects := lines(f.Git("log", "--format=%s", ref))
	if len(subjects) != 4 {
		t.Fatalf("commit %d개, want 4: %v", len(subjects), subjects)
	}
	for i, want := range []string{"done", "start", "claim", "create"} {
		if !strings.HasPrefix(subjects[i], want+":") {
			t.Fatalf("commit[%d] = %q, want %q 로 시작", i, subjects[i], want)
		}
	}

	// git 층: author 가 에이전트 ID 다. 사람 이름이 아니다.
	agent := f.agentID()
	for _, who := range lines(f.Git("log", "--format=%an", ref)) {
		if who != agent {
			t.Fatalf("author = %q, want %q", who, agent)
		}
	}

	// git 층: trailer 의 Status 가 issue.json 과 일치한다 (§3.3 의 전제).
	message := f.Git("log", "-1", "--format=%B", ref)
	if !strings.Contains(message, "Status: done") {
		t.Fatalf("trailer 에 Status: done 이 없습니다:\n%s", message)
	}
	blob := f.Git("cat-file", "-p", ref+":issue.json")
	if !strings.Contains(blob, `"status":"done"`) {
		t.Fatalf("issue.json 의 status 가 done 이 아닙니다:\n%s", blob)
	}
}

// E2E-1b: 배정 흐름 (오케스트레이터 모델).
//
// A 가 만들고 B 가 가져간다. A 가 B 몫으로 예약하는 경로는 없어야 한다 (§8.0).
func TestHandoffBetweenWorktrees(t *testing.T) {
	f := newFixture(t)
	a := f.AddWorktree("a", "feature/a")
	b := f.AddWorktree("b", "feature/b")
	idA := Identity{Agent: "agent-a", Session: "sa"}
	idB := Identity{Agent: "agent-b", Session: "sb"}

	id := issueID(t, f.RunJSONAs(idA, a.Path, "add", "작업1"))
	created := f.show(id)
	if created["status"] != "open" || created["owner"] != nil {
		t.Fatalf("add 직후 = %v, open/무소유 여야 합니다", created)
	}

	// start 가 claim 을 겸한다 (open → working).
	if r := f.RunAs(idB, b.Path, "start", id); r.ExitCode != 0 {
		t.Fatalf("B 의 start:\n%s", r)
	}
	if owner := f.show(id)["owner"]; owner != "agent-b" {
		t.Fatalf("owner = %v, want agent-b", owner)
	}
	// A 는 아무 소유권도 갖지 않는다.
	mine := f.RunJSONAs(idA, a.Path, "list", "--mine")
	if items, _ := mine.([]any); len(items) != 0 {
		t.Fatalf("A 의 --mine = %v, 비어 있어야 합니다", items)
	}

	if r := f.RunAs(idB, b.Path, "done", id); r.ExitCode != 0 {
		t.Fatalf("B 의 done:\n%s", r)
	}
	subjects := lines(f.Git("log", "--format=%s", "refs/ppwk/archive/"+id))
	if len(subjects) != 3 {
		t.Fatalf("commit %d개, want 3 (create/start/done): %v", len(subjects), subjects)
	}
}

// E2E-1c: 배정 대상 변경.
//
// B 가 응답하지 않아도 정리 명령 없이 C 가 곧바로 가져간다. 배정 상태를 ref 에
// 기록하는 설계라면 여기서 정리 책임이 생긴다.
func TestReassignmentNeedsNoCleanup(t *testing.T) {
	f := newFixture(t)
	a := f.AddWorktree("a", "feature/a")
	c := f.AddWorktree("c", "feature/c")

	id := issueID(t, f.RunJSONAs(Identity{Agent: "agent-a", Session: "sa"}, a.Path, "add", "작업1"))
	// agent-b 는 아무 명령도 실행하지 않는다.
	if status := f.show(id)["status"]; status != "open" {
		t.Fatalf("status = %v, want open", status)
	}
	if r := f.RunAs(Identity{Agent: "agent-c", Session: "sc"}, c.Path, "claim", id); r.ExitCode != 0 {
		t.Fatalf("C 의 claim:\n%s", r)
	}
	if owner := f.show(id)["owner"]; owner != "agent-c" {
		t.Fatalf("owner = %v, want agent-c", owner)
	}
	// B 의 흔적이 ref 에 없다.
	for name := range f.Refs("refs/ppwk/") {
		if strings.Contains(f.Git("log", "--format=%B", name), "agent-b") {
			t.Fatalf("%s 에 agent-b 의 흔적이 있습니다", name)
		}
	}
}

// E2E-2: 3 worktree 교차 가시성.
//
// 설계 §3.1 과 §14.4 의 전제를 직접 검증한다. commondir 해석이 깨지면 반드시
// 실패해야 한다.
func TestCrossWorktreeVisibility(t *testing.T) {
	f := newFixture(t)
	a := f.AddWorktree("a", "feature/a")
	b := f.AddWorktree("b", "feature/b")
	c := f.AddWorktree("c", "feature/c")

	// git 층: git-dir 은 서로 다르고 common-dir 은 같다.
	gitDirs := map[string]bool{}
	var commonDirs []string
	for _, wt := range []*Worktree{a, b, c} {
		gitDirs[strings.TrimSpace(f.GitIn(wt.Path, "rev-parse", "--absolute-git-dir"))] = true
		commonDirs = append(commonDirs,
			strings.TrimSpace(f.GitIn(wt.Path, "rev-parse", "--path-format=absolute", "--git-common-dir")))
	}
	if len(gitDirs) != 3 {
		t.Fatalf("git-dir 이 서로 다르지 않습니다: %v", gitDirs)
	}
	for _, dir := range commonDirs[1:] {
		if dir != commonDirs[0] {
			t.Fatalf("common-dir 이 다릅니다: %v", commonDirs)
		}
	}

	id := issueID(t, f.RunJSONAs(Identity{Agent: "agent-a", Session: "sa"}, a.Path, "add", "공유 작업"))

	// fetch 없이 B 에서 즉시 보인다.
	listed := f.RunJSONAs(Identity{Agent: "agent-b", Session: "sb"}, b.Path, "list")
	items, _ := listed.([]any)
	if len(items) != 1 {
		t.Fatalf("B 의 list = %v", listed)
	}

	if r := f.RunAs(Identity{Agent: "agent-c", Session: "sc"}, c.Path, "claim", id); r.ExitCode != 0 {
		t.Fatalf("C 의 claim:\n%s", r)
	}
	shown := f.RunJSONAs(Identity{Agent: "agent-a", Session: "sa"}, a.Path, "show", id)
	if owner := shown.(map[string]any)["owner"]; owner != "agent-c" {
		t.Fatalf("A 에서 본 owner = %v, want agent-c", owner)
	}

	// git 층: 어느 worktree 에서 봐도 OID 가 같다.
	ref := "refs/ppwk/issues/" + id
	var oids []string
	for _, wt := range []*Worktree{a, b, c} {
		oids = append(oids, strings.TrimSpace(f.GitIn(wt.Path, "rev-parse", ref)))
	}
	for _, oid := range oids[1:] {
		if oid != oids[0] {
			t.Fatalf("worktree 별 OID 가 다릅니다: %v", oids)
		}
	}
}

// E2E-3: 브랜치 독립성.
//
// 이슈는 브랜치 밖에 산다. 브랜치를 옮겨도 목록이 같고, 소스 이력에는
// 아무것도 남지 않는다.
func TestBranchIndependence(t *testing.T) {
	f := newFixture(t)
	a := f.AddWorktree("a", "feature/foo")
	b := f.AddWorktree("b", "feature/bar")

	idFoo := issueID(t, f.RunJSONAs(Identity{Agent: "a", Session: "sa"}, a.Path, "add", "foo 쪽 작업"))
	idBar := issueID(t, f.RunJSONAs(Identity{Agent: "b", Session: "sb"}, b.Path, "add", "bar 쪽 작업"))
	f.RunAs(Identity{Agent: "a", Session: "sa"}, a.Path, "claim", idFoo)

	before := f.RunJSON("list")
	// main 은 최상단 worktree 가 쥐고 있으므로 별도 브랜치로 옮긴다. 요점은
	// 어느 브랜치를 보든 이슈 목록이 같다는 것이다.
	f.gitIn(a.Path, "checkout", "--quiet", "-b", "other")
	after := f.RunJSON("list")
	if !sameJSON(before, after) {
		t.Fatalf("브랜치 전환으로 목록이 바뀌었습니다:\n%v\n%v", before, after)
	}

	// 워킹 디렉터리가 깨끗하고, 소스 이력에 이슈 commit 이 없다.
	f.expectCleanTree()
	for _, branch := range []string{"main", "feature/foo", "feature/bar"} {
		if subjects := f.Git("log", "--format=%s", branch); strings.Contains(subjects, idFoo) ||
			strings.Contains(subjects, idBar) {
			t.Fatalf("%s 이력에 이슈 commit 이 있습니다:\n%s", branch, subjects)
		}
	}
	if diff := f.Git("diff", "main", "feature/foo"); strings.TrimSpace(diff) != "" {
		t.Fatalf("브랜치 간 차이가 생겼습니다:\n%s", diff)
	}
}
