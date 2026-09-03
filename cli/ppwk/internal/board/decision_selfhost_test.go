package board

import (
	"fmt"
	"strings"
	"testing"
)

// 단계 12 exit criteria — 이 프로젝트 자신의 설계 결정을 담을 수 있는가.
//
// 스키마의 시금석이다. 실제 결정 열여덟 개를 담지 못하면 스키마가 부족한
// 것이다. 특히 두 가지를 본다:
//
//   - D8 → D9 → D11 의 폐기 체인이 --supersedes 로 표현되는가
//   - D10 (기각) 이 표현되는가 — 기각도 결정이다
func TestRecordsOwnDesignDecisions(t *testing.T) {
	b := initBoard(t)

	// spec/ppwk-decisions.md 를 압축한 것이다. 필드가 부족하면 여기서
	// 드러난다.
	type record struct {
		title        string
		context      string
		options      []string
		chosen       string
		consequences string
		supersedes   string
	}
	records := []record{
		{title: "저장 위치: custom ref namespace",
			context:      "여러 worktree 가 즉시 공유하고 소스 히스토리와 섞이지 않아야 한다",
			options:      []string{"tracked TASKS.json", "전용 브랜치", "git notes", "SQLite", "refs/ppwk/*"},
			chosen:       "refs/ppwk/*",
			consequences: "git log --all 에 이슈 커밋이 섞인다. --exclude 로 완화만 가능"},
		{title: "이슈당 ref 하나, 상태는 commit chain",
			options: []string{"단일 ref", "이슈당 ref"}, chosen: "이슈당 ref",
			consequences: "git log <ref> 가 곧 history"},
		{title: "CAS 는 git update-ref <ref> <new> <old>",
			options: []string{"go-git CheckAndSetReference", "git update-ref"}, chosen: "git update-ref",
			consequences: "문자열 매칭이 취약하다. 분류 실패 시 일반 오류로 둔다"},
		{title: "Agent-Session trailer 로 OID 충돌 방지",
			context: "content-addressed 라 같은 parent·tree·author·시각이면 OID 가 같아진다",
			options: []string{"세션 trailer 추가", "시각 정밀도 높이기"}, chosen: "세션 trailer 추가"},
		{title: "trailer 비정규화",
			options: []string{"매번 object 3개 읽기", "trailer 에 복제"}, chosen: "trailer 에 복제",
			consequences: "fsck 가 불일치를 검출한다"},
		{title: "plan 은 상태를 갖지 않는다",
			options: []string{"plan 에 진행률 저장", "task 에서 파생"}, chosen: "task 에서 파생"},
		{title: "go-git 하이브리드",
			options: []string{"순수 go-git", "순수 git CLI", "읽기 go-git + ref 갱신 exec"},
			chosen:  "읽기 go-git + ref 갱신 exec"},
		{title: "생존 판정 — TTL heartbeat (폐기)",
			options: []string{"TTL heartbeat"}, chosen: "TTL heartbeat",
			consequences: "데몬이 필요하고 유휴 구간이 사각지대가 된다"},
		{title: "생존 판정 — 세션 수명 동안 flock (폐기)",
			options: []string{"세션 수명 flock"}, chosen: "세션 수명 flock",
			consequences: "매 명령이 새 프로세스라 긴 잠금을 유지할 주체가 없다",
			supersedes:   "D008"},
		{title: "생존 판정 — 프로세스 트리 탐색 (기각)",
			context: "부모를 거슬러 올라가 claude / codex 이름을 찾는 안",
			options: []string{"프로세스 트리 탐색", "탐색하지 않음"}, chosen: "탐색하지 않음",
			consequences: "회귀 테스트: 코드에 프로세스 이름 조회가 존재하지 않아야 한다"},
		{title: "생존 판정 — hook_pid 또는 last_activity (현재)",
			options: []string{"hook_pid 또는 last_activity"}, chosen: "hook_pid 또는 last_activity",
			consequences: "훅 없는 환경에서 자동 회수는 사실상 하루 단위다",
			supersedes:   "D009"},
		{title: "세션 명령 없음",
			options: []string{"session begin/end/exec/status 유지", "전부 제거"}, chosen: "전부 제거"},
		{title: "lease ref 없음",
			options: []string{"refs/ppwk/agents/<name>", "잠금 파일만"}, chosen: "잠금 파일만"},
		{title: "배정은 오케스트레이터가",
			options: []string{"assign 명령과 assigned 상태", "메시지로 전달하고 open 유지"},
			chosen:  "메시지로 전달하고 open 유지"},
		{title: "SessionEnd 는 claimed 만 반납",
			options: []string{"전부 반납", "claimed 만 반납"}, chosen: "claimed 만 반납"},
		{title: "start 가 open 을 허용",
			options: []string{"claim 필수", "start 가 claim 을 겸함"}, chosen: "start 가 claim 을 겸함"},
		{title: "결정 기록을 도구 안에",
			options: []string{"--label decision 인 이슈", "refs/ppwk/decisions/ 에 불변 ADR"},
			chosen:  "refs/ppwk/decisions/ 에 불변 ADR"},
		{title: "백로그는 상태가 아니라 priority none",
			options: []string{"backlog 상태 추가", "priority none"}, chosen: "priority none",
			consequences: "next 필터 한 줄. 전이·gate·회수 규칙에 예외가 없다"},
	}

	for i, r := range records {
		decision, err := b.Decide(DecideOptions{
			Title: r.title, Context: r.context, Options: r.options,
			Chosen: r.chosen, Consequences: r.consequences, Supersedes: r.supersedes,
		})
		if err != nil {
			t.Fatalf("D%d(%s) 를 기록할 수 없습니다: %v", i+1, r.title, err)
		}
		if want := formatDecisionID(i + 1); decision.ID != want {
			t.Fatalf("%d번째 ID = %s, want %s", i+1, decision.ID, want)
		}
	}

	// D8 → D9 → D11 폐기 체인.
	chain, err := b.DecisionHistory("D011")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(chain))
	for _, decision := range chain {
		got = append(got, decision.ID)
	}
	if fmt.Sprint(got) != "[D011 D009 D008]" {
		t.Fatalf("폐기 체인 = %v", got)
	}

	// 폐기된 둘은 유효한 목록에서 빠진다. 기각(D10)은 폐기가 아니므로 남는다.
	entries, err := b.ListDecisions(DecisionListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	valid := map[string]bool{}
	for _, entry := range entries {
		valid[entry.ID] = true
	}
	if valid["D008"] || valid["D009"] {
		t.Fatalf("폐기된 결정이 유효 목록에 있습니다: %v", decisionIDs(entries))
	}
	if !valid["D010"] || !valid["D011"] {
		t.Fatalf("유효 목록 = %v", decisionIDs(entries))
	}
	if len(entries) != len(records)-2 {
		t.Fatalf("유효 결정 %d개, want %d", len(entries), len(records)-2)
	}

	// D10 은 기각이다 — 검토한 선택지를 택하지 않았다는 사실이 담겨야 한다.
	rejected, err := b.ShowDecision("D010")
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Chosen != "탐색하지 않음" || len(rejected.Options) != 2 {
		t.Fatalf("D010 = %+v", rejected.Decision)
	}
	if !strings.Contains(rejected.Title, "기각") {
		t.Fatalf("D010 제목 = %q", rejected.Title)
	}

	// 무결성 검사가 깨끗해야 한다 — 매달린 참조도 순환도 없다.
	findings, err := b.Fsck(FsckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("fsck = %v", findings)
	}
}
