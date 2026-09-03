package board

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/refstore"
)

func mustDecide(t *testing.T, b *Board, opts DecideOptions) *Decision {
	t.Helper()
	decision, err := b.Decide(opts)
	if err != nil {
		t.Fatalf("Decide(%q) = %v", opts.Title, err)
	}
	return decision
}

func decisionIDs(entries []DecisionEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.ID)
	}
	return out
}

// T12.1 decide 가 ref 를 만들고 ID 를 채번한다.
func TestDecideCreatesRef(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})

	decision := mustDecide(t, b, DecideOptions{
		Title: "저장소는 SQLite", Context: "단일 머신",
		Options: []string{"SQLite", "PostgreSQL"}, Chosen: "SQLite",
		Consequences: "동시 쓰기 확장 시 재검토",
		Issues:       []string{issue.ID},
		Body:         []byte("긴 근거\n"),
	})
	if decision.ID != "D001" {
		t.Fatalf("ID = %q", decision.ID)
	}
	if !refExists(t, b, refstore.Decisions+"D001") {
		t.Fatal("ref 가 없습니다")
	}
	if decision.DecidedBy != b.identity.Agent || decision.DecidedAt.Time.IsZero() {
		t.Fatalf("decision = %+v", decision.Decision)
	}

	read, err := b.ShowDecision("D001")
	if err != nil {
		t.Fatal(err)
	}
	if read.Chosen != "SQLite" || string(read.Body) != "긴 근거\n" {
		t.Fatalf("read = %+v body=%q", read.Decision, read.Body)
	}
	if fmt.Sprint(read.Issues) != fmt.Sprint([]string{issue.ID}) {
		t.Fatalf("issues = %v", read.Issues)
	}

	// 두 번째는 D002 다.
	if second := mustDecide(t, b, DecideOptions{Title: "둘"}); second.ID != "D002" {
		t.Fatalf("두 번째 ID = %q", second.ID)
	}
}

// T12.3 수정 명령이 존재하지 않는다.
//
// 불변이라는 성질이 이 기능의 전부다. "편의를 위해" edit 을 더하고 싶어지는
// 지점이라 회귀로 못박는다.
func TestNoDecisionMutationMethods(t *testing.T) {
	banned := []string{
		"func (b *Board) EditDecision", "func (b *Board) UpdateDecision",
		"func (b *Board) MutateDecision", "func (b *Board) DeleteDecision",
		"func (b *Board) SetDecision",
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, signature := range banned {
			if strings.Contains(string(data), signature) {
				t.Fatalf("%s 에 %q 가 있습니다 — 결정은 불변입니다 (§3.9)", name, signature)
			}
		}
	}
}

// T12.4 --supersedes 는 새 결정을 만들고 이전 결정을 건드리지 않는다.
func TestSupersedeLeavesPredecessorUntouched(t *testing.T) {
	b := initBoard(t)
	first := mustDecide(t, b, DecideOptions{Title: "파일 기반 JSON", Chosen: "JSON",
		Options: []string{"JSON"}})
	before, err := b.ShowDecision(first.ID)
	if err != nil {
		t.Fatal(err)
	}

	second := mustDecide(t, b, DecideOptions{Title: "저장소는 SQLite", Chosen: "SQLite",
		Options: []string{"SQLite"}, Supersedes: first.ID})
	if second.Supersedes != first.ID {
		t.Fatalf("supersedes = %q", second.Supersedes)
	}

	after, err := b.ShowDecision(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Commit != before.Commit {
		t.Fatal("이전 결정이 다시 쓰였습니다")
	}
}

// T12.5 superseded_by 는 저장되지 않고 조회 시 계산된다.
func TestSupersededByIsDerived(t *testing.T) {
	b := initBoard(t)
	first := mustDecide(t, b, DecideOptions{Title: "첫째"})
	second := mustDecide(t, b, DecideOptions{Title: "둘째", Supersedes: first.ID})

	// 문서 어디에도 superseded_by 가 없다.
	raw, err := model.Marshal(first.Decision)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "superseded_by") {
		t.Fatalf("문서에 저장됐습니다: %s", raw)
	}

	entries, err := b.ListDecisions(DecisionListOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %v", decisionIDs(entries))
	}
	if fmt.Sprint(entries[0].SupersededBy) != fmt.Sprint([]string{second.ID}) {
		t.Fatalf("superseded_by = %v", entries[0].SupersededBy)
	}
	if entries[1].Superseded() {
		t.Fatal("최신 결정이 superseded 로 표시됐습니다")
	}
}

// T12.6 기본 목록은 superseded 를 제외한다.
func TestListExcludesSuperseded(t *testing.T) {
	b := initBoard(t)
	first := mustDecide(t, b, DecideOptions{Title: "첫째"})
	second := mustDecide(t, b, DecideOptions{Title: "둘째", Supersedes: first.ID})

	entries, err := b.ListDecisions(DecisionListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(decisionIDs(entries)) != fmt.Sprint([]string{second.ID}) {
		t.Fatalf("기본 목록 = %v", decisionIDs(entries))
	}

	all, err := b.ListDecisions(DecisionListOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("--all = %v", decisionIDs(all))
	}
}

// T12.7 --issue 는 연결된 것만 돌려준다.
func TestListByIssue(t *testing.T) {
	b := initBoard(t)
	first := mustAdd(t, b, AddOptions{Title: "하나"})
	second := mustAdd(t, b, AddOptions{Title: "둘"})
	linked := mustDecide(t, b, DecideOptions{Title: "연결됨", Issues: []string{first.ID, second.ID}})
	mustDecide(t, b, DecideOptions{Title: "무관"})

	entries, err := b.ListDecisions(DecisionListOptions{Issue: first.ID})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(decisionIDs(entries)) != fmt.Sprint([]string{linked.ID}) {
		t.Fatalf("--issue = %v", decisionIDs(entries))
	}
}

// T12.8 이슈에서 연결된 결정을 찾을 수 있다. archive 에 있어도 마찬가지다.
func TestDecisionsForArchivedIssue(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상"})
	decision := mustDecide(t, b, DecideOptions{Title: "결정", Issues: []string{issue.ID}})
	transitionAll(t, b, issue.ID, ActionStart, ActionDone)

	entries, err := b.DecisionsForIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(decisionIDs(entries)) != fmt.Sprint([]string{decision.ID}) {
		t.Fatalf("= %v", decisionIDs(entries))
	}
	// 연결 이슈가 archive 로 갔다고 fsck 가 매달린 참조라고 하면 안 된다.
	if got := findingsFor(t, b, CheckDecisionRef); len(got) != 0 {
		t.Fatalf("fsck = %v", got)
	}
}

// --plan 과 --search 는 trailer 에 없는 것을 본다.
func TestListByPlanAndSearch(t *testing.T) {
	b := initBoard(t)
	makePlan(t, b, "계획", model.PriorityMed, model.Phase{ID: "p1", Title: "하나", Gate: model.GateAllDone})
	inPlan := mustDecide(t, b, DecideOptions{Title: "계획 소속", Plan: "P01"})
	mustDecide(t, b, DecideOptions{Title: "무관", Body: []byte("여기 열쇠말이 있다\n")})

	entries, err := b.ListDecisions(DecisionListOptions{Plan: "P01"})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(decisionIDs(entries)) != fmt.Sprint([]string{inPlan.ID}) {
		t.Fatalf("--plan = %v", decisionIDs(entries))
	}

	// 본문도 훑는다.
	found, err := b.ListDecisions(DecisionListOptions{Search: "열쇠말"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Title != "무관" {
		t.Fatalf("--search = %v", decisionIDs(found))
	}
}

// T12.9 export --decisions 는 결정당 파일 하나를 만들고 파생물 헤더를 넣는다.
func TestExportDecisions(t *testing.T) {
	b := initBoard(t)
	first := mustDecide(t, b, DecideOptions{Title: "첫째", Chosen: "A", Options: []string{"A", "B"}})
	second := mustDecide(t, b, DecideOptions{Title: "둘째", Supersedes: first.ID})

	dir := filepath.Join(t.TempDir(), "docs", "decisions")
	written, err := b.ExportDecisions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 2 {
		t.Fatalf("파일 %d개: %v", len(written), written)
	}

	data, err := os.ReadFile(filepath.Join(dir, first.ID+".md"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, DerivedWarning) || !strings.Contains(out, first.DecidedAt.String()) {
		t.Fatalf("헤더가 없습니다:\n%s", out)
	}
	if !strings.Contains(out, "Superseded by: "+second.ID) {
		t.Fatalf("파생 역방향 엣지가 없습니다:\n%s", out)
	}
	if !strings.Contains(out, "**(택함)** A") {
		t.Fatalf("택한 선택지 표시가 없습니다:\n%s", out)
	}

	// 이미 있는 파일은 덮어쓴다. 파생물이므로 손으로 고친 것을 지키지 않는다.
	if _, err := b.ExportDecisions(dir); err != nil {
		t.Fatal(err)
	}
}

// T12.10 fsck 가 없는 issue / plan / supersedes 참조를 검출한다.
func TestFsckDetectsDanglingDecisionRefs(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts DecideOptions
	}{
		{"issue", DecideOptions{Title: "d", Issues: []string{"T999"}}},
		{"plan", DecideOptions{Title: "d", Plan: "P99"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := initBoard(t)
			decision := mustDecide(t, b, tc.opts)
			expectFinding(t, b, CheckDecisionRef, decision.ID, LevelError)
		})
	}

	// supersedes 는 Decide 가 미리 막는다.
	t.Run("supersedes 는 생성 시 거부", func(t *testing.T) {
		b := initBoard(t)
		if _, err := b.Decide(DecideOptions{Title: "d", Supersedes: "D999"}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Decide() = %v, want ErrNotFound", err)
		}
	})

	// 그래도 사슬이 끊긴 데이터는 있을 수 있다 — ref 를 지우면 그렇게 된다.
	t.Run("supersedes 사슬 끊김", func(t *testing.T) {
		b := initBoard(t)
		first := mustDecide(t, b, DecideOptions{Title: "첫째"})
		second := mustDecide(t, b, DecideOptions{Title: "둘째", Supersedes: first.ID})
		hash, err := b.Store().Get(refstore.Decisions + first.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := b.Store().CAS(refstore.Decisions+first.ID, plumbing.ZeroHash, hash); err != nil {
			t.Fatal(err)
		}
		expectFinding(t, b, CheckDecisionRef, second.ID, LevelError)
	})
}

// 자기 자신을 대체할 수 없다.
func TestSelfSupersedeRejected(t *testing.T) {
	b := initBoard(t)
	first := mustDecide(t, b, DecideOptions{Title: "첫째"})
	if _, err := b.Decide(DecideOptions{Title: "둘째", Supersedes: first.ID}); err != nil {
		t.Fatal(err)
	}
	// D002 가 D002 를 대체하는 것은 만들 수 없다 — 채번이 먼저이므로
	// Validate 가 걸러낸다.
	decision := model.Decision{ID: "D003", Title: "t", Supersedes: "D003"}
	if err := decision.Validate(); err == nil {
		t.Fatal("자기 자신 대체가 통과했습니다")
	}
}

// 이미 superseded 된 결정을 다시 대체하는 것은 허용된다 (분기).
func TestBranchingSupersedeAllowed(t *testing.T) {
	b := initBoard(t)
	base := mustDecide(t, b, DecideOptions{Title: "기반"})
	left := mustDecide(t, b, DecideOptions{Title: "왼쪽", Supersedes: base.ID})
	right := mustDecide(t, b, DecideOptions{Title: "오른쪽", Supersedes: base.ID})

	entries, err := b.ListDecisions(DecisionListOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(entries[0].SupersededBy) != fmt.Sprint([]string{left.ID, right.ID}) {
		t.Fatalf("superseded_by = %v", entries[0].SupersededBy)
	}
	// history 는 한 방향만 따라간다.
	chain, err := b.DecisionHistory(right.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 2 || chain[0].ID != right.ID || chain[1].ID != base.ID {
		t.Fatalf("chain = %v", chain)
	}
}

// D1~D16 같은 폐기 체인이 표현되는지 본다 (단계 12 exit criteria).
func TestSupersedeChainOfThree(t *testing.T) {
	b := initBoard(t)
	d8 := mustDecide(t, b, DecideOptions{Title: "D8 안", Chosen: "A", Options: []string{"A"}})
	d9 := mustDecide(t, b, DecideOptions{Title: "D9 안", Chosen: "B", Options: []string{"B"},
		Supersedes: d8.ID})
	d11 := mustDecide(t, b, DecideOptions{Title: "D11 안", Chosen: "C", Options: []string{"C"},
		Supersedes: d9.ID})

	chain, err := b.DecisionHistory(d11.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(chain))
	for _, decision := range chain {
		got = append(got, decision.ID)
	}
	if fmt.Sprint(got) != fmt.Sprint([]string{d11.ID, d9.ID, d8.ID}) {
		t.Fatalf("chain = %v", got)
	}
	// 유효한 것은 마지막 하나뿐이다.
	entries, err := b.ListDecisions(DecisionListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(decisionIDs(entries)) != fmt.Sprint([]string{d11.ID}) {
		t.Fatalf("유효한 결정 = %v", decisionIDs(entries))
	}
}

// 기각도 결정이다. options 에 없는 것을 택한 것으로 기록해도 막지 않는다.
func TestRejectionIsADecision(t *testing.T) {
	b := initBoard(t)
	decision := mustDecide(t, b, DecideOptions{
		Title: "TTL 방식 기각", Options: []string{"TTL", "잠금"}, Chosen: "잠금",
		Consequences: "TTL 은 도입하지 않는다",
	})
	if decision.Chosen != "잠금" {
		t.Fatalf("chosen = %q", decision.Chosen)
	}
	// options 밖의 값도 허용된다 — 사후에 추가된 선택지일 수 있다.
	loose := mustDecide(t, b, DecideOptions{Title: "나중 안", Options: []string{"A"}, Chosen: "C"})
	if loose.Chosen != "C" {
		t.Fatalf("chosen = %q", loose.Chosen)
	}
	if got := findingsFor(t, b, CheckDecisionRef); len(got) != 0 {
		t.Fatalf("fsck = %v", got)
	}
}

// F12.2 임의 supersedes 그래프에서 순환 검출이 panic 없이 끝난다.
func FuzzSupersedesChain(f *testing.F) {
	f.Add(uint64(1))
	f.Add(uint64(77))
	f.Fuzz(func(t *testing.T, seed uint64) {
		scan := generateDecisions(seed)
		findings := checkSupersedes(scan)
		for _, found := range findings {
			if found.Check != CheckSupersedesCycle || found.Level != LevelError {
				t.Fatalf("finding = %+v", found)
			}
		}
		// 순환이 없다고 판정했다면 모든 사슬이 실제로 끝나야 한다.
		if len(findings) == 0 {
			for _, id := range scan.decisionOrder {
				steps := 0
				for current := id; current != ""; {
					decision, ok := scan.decisions[current]
					if !ok {
						break
					}
					current = decision.Supersedes
					if steps++; steps > len(scan.decisions)+1 {
						t.Fatalf("%s 의 사슬이 끝나지 않는데 순환이 보고되지 않았습니다", id)
					}
				}
			}
		}
	})
}

// generateDecisions 는 seed 로 supersedes 그래프를 만든다. 순환과 매달린
// 참조가 자연스럽게 섞인다.
func generateDecisions(seed uint64) *fsckScan {
	rng := rand.New(rand.NewPCG(seed, seed^0x2545f491))
	scan := &fsckScan{decisions: map[string]model.Decision{}}
	n := rng.IntN(12)
	for i := range n {
		id := formatDecisionID(i + 1)
		decision := model.Decision{Schema: 1, ID: id, Title: "t"}
		// 앞뒤를 가리지 않고 고르므로 순환이 생긴다. 없는 ID 도 섞는다.
		switch rng.IntN(4) {
		case 0:
		case 1:
			decision.Supersedes = formatDecisionID(1 + rng.IntN(n+2))
		default:
			decision.Supersedes = formatDecisionID(1 + rng.IntN(max(i, 1)))
		}
		if decision.Supersedes == id {
			decision.Supersedes = ""
		}
		scan.decisions[id] = decision
		scan.decisionOrder = append(scan.decisionOrder, id)
	}
	return scan
}

// T12.12 결정은 3개 worktree 에서 즉시 공유된다. 브랜치와 무관하다.
//
// 결정을 tracked 파일로 기록하면 브랜치마다 갈린다 — TASKS.json 을 기각한
// 이유(D1)와 같은 문제이고, ref namespace 가 같은 해법이다 (§3.9).
func TestDecisionsVisibleAcrossWorktrees(t *testing.T) {
	b, main := initBoardDir(t)
	decision := mustDecide(t, b, DecideOptions{Title: "저장소는 SQLite",
		Options: []string{"SQLite"}, Chosen: "SQLite"})

	// worktree add 는 커밋된 브랜치를 요구한다.
	runGit(t, main, "commit", "--allow-empty", "--quiet", "-m", "base")

	base := t.TempDir()
	for i := range 3 {
		path := filepath.Join(base, fmt.Sprintf("wt%d", i))
		runGit(t, main, "worktree", "add", "--quiet", "-b", fmt.Sprintf("b%d", i), path)

		linked, err := Open(path, b.Identity())
		if err != nil {
			t.Fatalf("worktree %d: Open() = %v", i, err)
		}
		entries, err := linked.ListDecisions(DecisionListOptions{})
		if err != nil {
			t.Fatalf("worktree %d: %v", i, err)
		}
		if fmt.Sprint(decisionIDs(entries)) != fmt.Sprint([]string{decision.ID}) {
			t.Fatalf("worktree %d: %v", i, decisionIDs(entries))
		}
	}

	// 다른 worktree 에서 만든 결정도 즉시 보인다.
	other, err := Open(filepath.Join(base, "wt0"), b.Identity())
	if err != nil {
		t.Fatal(err)
	}
	added := mustDecide(t, other, DecideOptions{Title: "저쪽에서 만든 결정"})

	entries, err := b.ListDecisions(DecisionListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(decisionIDs(entries)) != fmt.Sprint([]string{decision.ID, added.ID}) {
		t.Fatalf("본 저장소에서 = %v", decisionIDs(entries))
	}
}

// T12.11 — fsck 가 supersedes 순환을 검출한다.
//
// 정상 경로로는 만들 수 없다 (결정은 불변이고 Decide 가 대상의 실재를
// 확인한다). 그래도 손상된 저장소에서 DecisionHistory 가 무한 루프에
// 빠지지 않으려면 검출이 필요하다. F12.2 는 임의 그래프에서 panic 이
// 없음을 보고, 여기서는 최소 순환이 실제로 보고되는지를 본다.
func TestFsckDetectsSupersedesCycle(t *testing.T) {
	for _, tc := range []struct {
		name  string
		graph map[string]string // id → supersedes
		want  int
	}{
		{"자기 참조", map[string]string{"D001": "D001"}, 1},
		{"2-순환", map[string]string{"D001": "D002", "D002": "D001"}, 1},
		{"3-순환", map[string]string{"D001": "D002", "D002": "D003", "D003": "D001"}, 1},
		{"꼬리가 붙은 순환", map[string]string{
			"D001": "D002", "D002": "D003", "D003": "D002"}, 1},
		{"순환 없음", map[string]string{"D001": "D002", "D002": ""}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scan := &fsckScan{decisions: map[string]model.Decision{}}
			for id, next := range tc.graph {
				scan.decisions[id] = model.Decision{ID: id, Supersedes: next}
				scan.decisionOrder = append(scan.decisionOrder, id)
			}
			slices.Sort(scan.decisionOrder)

			findings := checkSupersedes(scan)
			if len(findings) != tc.want {
				t.Fatalf("발견 %d건, want %d: %v", len(findings), tc.want, findings)
			}
			for _, found := range findings {
				if found.Check != CheckSupersedesCycle || found.Level != LevelError {
					t.Fatalf("finding = %+v", found)
				}
			}
		})
	}
}
