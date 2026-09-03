package e2e

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// E2E-14: phase 진행.
//
// gate 는 저장하지 않고 매번 파생한다 (§3.7.1). 그래서 task 하나가 끝날
// 때마다 다음 phase 의 개방 여부가 저절로 바뀐다.
func TestPhaseProgression(t *testing.T) {
	f := newFixture(t)
	f.MustRun("plan", "new", "P", "--id", "P01")
	for _, phase := range []struct{ id, gate string }{
		{"p1", "all_done"}, {"p2", "all_done"}, {"p3", "manual"},
	} {
		f.MustRun("plan", "phase", "add", "P01", phase.id, "--id", phase.id, "--gate", phase.gate)
	}
	tasks := map[string][]string{}
	for _, phase := range []string{"p1", "p2", "p3"} {
		for i := range 2 {
			id := f.add(fmt.Sprintf("%s 작업 %d", phase, i),
				"--plan", "P01", "--phase", phase, "--seq", fmt.Sprint((i+1)*10))
			tasks[phase] = append(tasks[phase], id)
		}
	}

	// p1 만 후보다.
	f.expectCandidates(tasks["p1"])
	f.expectProgress("P01", 0, 6, "p1")

	// p1 을 하나만 끝내면 아직 p2 는 막혀 있다 (all_done).
	f.finish(tasks["p1"][0])
	f.expectCandidates(tasks["p1"][1:])
	f.expectProgress("P01", 1, 6, "p1")

	// p1 이 전부 끝나면 p2 가 열린다.
	f.finish(tasks["p1"][1])
	f.expectCandidates(tasks["p2"])
	f.expectProgress("P01", 2, 6, "p2")

	// p2 를 끝내도 p3 는 manual 이라 막혀 있다.
	f.finish(tasks["p2"][0])
	f.finish(tasks["p2"][1])
	f.expectCandidates(nil)

	f.MustRun("plan", "advance", "P01", "p3")
	f.expectCandidates(tasks["p3"])
	f.expectProgress("P01", 4, 6, "p3")
}

// E2E-15: plan 경쟁 분산.
//
// plan ref 쓰기 0회가 핵심이다 (§3.7.1). plan 에 진행률 필드를 추가하면
// 반드시 실패한다 — 그 필드가 경쟁 지점이 되기 때문이다.
func TestPlanRefIsNeverWrittenDuringWork(t *testing.T) {
	f := newFixture(t)
	f.MustRun("plan", "new", "P", "--id", "P01")
	f.MustRun("plan", "phase", "add", "P01", "p1", "--id", "p1")
	for i := range 10 {
		f.add(fmt.Sprintf("작업 %02d", i), "--plan", "P01", "--phase", "p1",
			"--seq", fmt.Sprint((i+1)*10))
	}
	const agents = 8
	var dirs []string
	for i := range agents {
		dirs = append(dirs, f.AddWorktree(fmt.Sprintf("a%d", i), fmt.Sprintf("br/%d", i)).Path)
	}
	before := f.Refs("refs/ppwk/plans/")["refs/ppwk/plans/P01"]

	claimed := make([]string, agents)
	race(f, agents, func(i int, ident Identity) Result {
		r := f.RunAs(ident, dirs[i], "--json", "next", "--claim")
		claimed[i] = claimedID(r.Stdout)
		return r
	})

	seen := map[string]int{}
	for i, id := range claimed {
		if id == "" {
			continue
		}
		if prev, dup := seen[id]; dup {
			t.Fatalf("%s 가 agent-%02d 와 agent-%02d 에게 중복 배정됐습니다\n%s",
				id, prev, i, f.issueTimeline(id))
		}
		seen[id] = i
	}

	after := f.Refs("refs/ppwk/plans/")["refs/ppwk/plans/P01"]
	if after != before {
		t.Fatalf("plan ref 가 %s → %s 로 바뀌었습니다 — plan 은 작업 중에 쓰이지 않아야 합니다",
			before, after)
	}
}

// E2E-16: seq 우선 정렬.
//
// 같은 phase 안에서는 seq 가 priority 보다 앞선다 (§7.2). 계획한 순서가
// 개별 이슈의 급함보다 우선한다는 뜻이다.
func TestSeqBeatsPriorityWithinPhase(t *testing.T) {
	f := newFixture(t)
	f.MustRun("plan", "new", "P", "--id", "P01")
	f.MustRun("plan", "phase", "add", "P01", "p1", "--id", "p1")
	early := f.add("먼저 할 것", "--plan", "P01", "--phase", "p1", "--seq", "10", "--priority", "low")
	late := f.add("급하지만 나중", "--plan", "P01", "--phase", "p1", "--seq", "20", "--priority", "high")

	if got := f.candidates(); len(got) == 0 || got[0] != early {
		t.Fatalf("후보 = %v, %s (seq 10) 가 먼저여야 합니다 (%s 는 seq 20)", got, early, late)
	}
}

// E2E-17: 의존성과 archive.
//
// issues/ 만 조회하는 구현이면 T002 가 영원히 후보에 안 나온다. done 이
// archive 이동을 겸하기 때문이다.
func TestDependencySatisfiedFromArchive(t *testing.T) {
	f := newFixture(t)
	first := f.add("선행")
	second := f.add("후속", "--depends-on", first)

	if got := f.candidates(); slices.Contains(got, second) {
		t.Fatalf("선행이 안 끝났는데 %s 가 후보입니다: %v", second, got)
	}
	f.finish(first)
	if !f.HasRef("refs/ppwk/archive/" + first) {
		t.Fatalf("%s 가 archive 로 가지 않았습니다", first)
	}
	if got := f.candidates(); !slices.Contains(got, second) {
		t.Fatalf("후보 = %v, %s 가 있어야 합니다 — 의존 검사가 archive 를 봐야 합니다", got, second)
	}
}

// E2E-17b: 브랜치 간 결정 공유.
//
// 결정을 tracked 파일로 두면 이 시나리오가 실패한다 (§3.9).
func TestDecisionsAreBranchIndependent(t *testing.T) {
	f := newFixture(t)
	a := f.AddWorktree("a", "feature/a")
	b := f.AddWorktree("b", "feature/b")
	idA := Identity{Agent: "agent-a", Session: "sa"}
	idB := Identity{Agent: "agent-b", Session: "sb"}

	task := issueID(t, f.RunJSONAs(idA, a.Path, "add", "저장소 고르기"))
	decision := f.RunJSONAs(idA, a.Path, "decide", "저장소는 SQLite",
		"--option", "SQLite", "--option", "Postgres", "--decision", "SQLite", "--issue", task)
	did := fmt.Sprint(decision.(map[string]any)["id"])

	// merge 없이 B 에서 즉시 보인다.
	items, _ := f.RunJSONAs(idB, b.Path, "decisions").([]any)
	if len(items) != 1 || fmt.Sprint(items[0].(map[string]any)["id"]) != did {
		t.Fatalf("B 에서 본 결정 = %v", items)
	}
	shown := f.RunJSONAs(idB, b.Path, "decisions", "--issue", task)
	if list, _ := shown.([]any); len(list) != 1 {
		t.Fatalf("이슈에 연결된 결정 = %v", shown)
	}

	// 소스 이력 어디에도 결정 커밋이 없다.
	for _, branch := range []string{"main", "feature/a", "feature/b"} {
		if log := f.Git("log", "--format=%s", branch); strings.Contains(log, did) ||
			strings.Contains(log, "저장소는 SQLite") {
			t.Fatalf("%s 이력에 결정이 있습니다:\n%s", branch, log)
		}
	}
	f.expectCleanTree()
}

// E2E-17c: supersede 체인.
//
// 결정은 불변이다 (§3.9). 대체는 새 결정을 만들 뿐, 이전 것을 건드리지 않는다.
func TestSupersedeChain(t *testing.T) {
	f := newFixture(t)
	d1 := f.decide("첫 결정", "--decision", "A")
	oid1 := f.Refs("refs/ppwk/decisions/")["refs/ppwk/decisions/"+d1]

	d2 := f.decide("둘째 결정", "--decision", "B", "--supersedes", d1)
	d3 := f.decide("셋째 결정", "--decision", "C", "--supersedes", d2)

	if got := f.decisionIDs(); !slices.Equal(got, []string{d3}) {
		t.Fatalf("기본 목록 = %v, want [%s]", got, d3)
	}
	if got := f.decisionIDs("--all"); len(got) != 3 {
		t.Fatalf("--all = %v, want 3건", got)
	}

	history, _ := f.RunJSON("decisions", "history", d3).([]any)
	var chain []string
	for _, item := range history {
		chain = append(chain, fmt.Sprint(item.(map[string]any)["id"]))
	}
	if !slices.Equal(chain, []string{d3, d2, d1}) {
		t.Fatalf("체인 = %v, want [%s %s %s]", chain, d3, d2, d1)
	}

	// D001 의 ref 는 처음과 같다. 대체당해도 자기 자신은 바뀌지 않는다.
	if after := f.Refs("refs/ppwk/decisions/")["refs/ppwk/decisions/"+d1]; after != oid1 {
		t.Fatalf("%s 의 OID 가 %s → %s 로 바뀌었습니다 — 결정은 불변이어야 합니다", d1, oid1, after)
	}
}

// E2E-17d: export 후 커밋.
//
// 파생물을 커밋하는 것은 정상적인 사용이다. 다만 그것이 단방향임을 파일이
// 스스로 밝혀야 한다.
func TestExportDecisionsThenCommit(t *testing.T) {
	f := newFixture(t)
	d1 := f.decide("첫 결정", "--decision", "A", "--context", "배경")
	d2 := f.decide("둘째 결정", "--decision", "B")

	out := "docs/decisions"
	f.MustRun("export", "--decisions", "-o", out)
	files := lines(f.GitIn(f.Root, "status", "--porcelain", "--untracked-files=all", out))
	if len(files) != 2 {
		t.Fatalf("파일 %d개, want 2 (결정당 하나): %v", len(files), files)
	}

	for _, id := range []string{d1, d2} {
		body := f.readExported(out, id)
		if !strings.Contains(body, "파생물") {
			t.Fatalf("%s 파일 헤더에 파생물 경고가 없습니다:\n%s", id, body)
		}
		shown, _ := f.RunJSON("decisions", "show", id).(map[string]any)
		decision, _ := shown["decision"].(map[string]any)
		title := fmt.Sprint(decision["title"])
		if !strings.Contains(body, title) {
			t.Fatalf("%s 파일 내용이 ref 와 다릅니다:\n%s", id, body)
		}
	}

	f.Git("add", out)
	f.Git("commit", "--quiet", "-m", "결정 기록 내보내기")
	f.expectCleanTree()
}
