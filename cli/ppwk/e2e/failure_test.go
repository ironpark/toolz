package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// E2E-8: 에이전트 SIGKILL 후 즉시 회수.
//
// TTL 대기가 없다. 이전 설계에서는 최대 10분을 기다려야 했다 (§3.6).
func TestKilledAgentIsReapedImmediately(t *testing.T) {
	f := newFixture(t)
	victimDir := f.AddWorktree("victim", "br/victim").Path
	otherDir := f.AddWorktree("other", "br/other").Path

	id := f.add("피해자의 작업")
	victim := f.Agent("agent-b", victimDir)
	if r := victim.Run("start", id); r.ExitCode != 0 {
		t.Fatalf("start:\n%s", r)
	}
	f.expectStatus(id, "working")
	before := len(lines(f.Git("log", "--format=%H", "refs/ppwk/issues/"+id)))

	victim.Kill()

	// 대기 없이, 다음 next 호출이 회수한다.
	// --dry-run 은 저장소를 변형하지 않으므로 회수도 하지 않는다. 회수는
	// 변형하는 next 에 딸려 오는 것이 설계다 (§4.5).
	rescuer := Identity{Agent: "agent-c", Session: "sc"}
	if r := f.RunAs(rescuer, otherDir, "next"); r.ExitCode != 0 {
		t.Fatalf("next:\n%s", r)
	}
	if status := f.show(id)["status"]; status != "open" {
		t.Fatalf("status = %v, want open (즉시 회수돼야 합니다)", status)
	}

	// git 층: 회수도 이력이다. 지우지 않고 덧붙인다.
	after := lines(f.Git("log", "--format=%s", "refs/ppwk/issues/"+id))
	if len(after) != before+1 {
		t.Fatalf("commit 이 %d → %d, 회수 commit 1개가 늘어야 합니다", before, len(after))
	}
	if !strings.HasPrefix(after[0], "reap:") {
		t.Fatalf("최신 commit = %q, reap 이어야 합니다", after[0])
	}
	if who := strings.TrimSpace(f.Git("log", "-1", "--format=%an", "refs/ppwk/issues/"+id)); who != "agent-c" {
		t.Fatalf("회수 commit 의 author = %q, want agent-c", who)
	}

	// 곧바로 재배정된다.
	if r := f.RunAs(rescuer, otherDir, "claim", id); r.ExitCode != 0 {
		t.Fatalf("재배정:\n%s", r)
	}
}

// E2E-9: 유휴 상태에서도 손실 없음.
//
// 게으른 확인의 정당성이다 (§4.5). 아무도 원하지 않는 동안 방치되어도
// 손해가 없고, 필요해진 순간에 확인된다.
func TestIdleBoardLosesNothing(t *testing.T) {
	f := newFixture(t)
	var agents []*Agent
	var ids []string
	for i := range 3 {
		dir := f.AddWorktree(fmt.Sprintf("w%d", i), fmt.Sprintf("br/%d", i)).Path
		id := f.add(fmt.Sprintf("작업 %d", i))
		a := f.Agent(fmt.Sprintf("agent-%d", i), dir)
		if r := a.Run("start", id); r.ExitCode != 0 {
			t.Fatalf("start:\n%s", r)
		}
		agents = append(agents, a)
		ids = append(ids, id)
	}

	agents[1].Kill()

	// 아무도 next 를 부르지 않는 구간. 시간이 흘러도 아무 일도 일어나지 않는다.
	for _, id := range ids {
		if status := f.show(id)["status"]; status != "working" {
			t.Fatalf("%s = %v, 아무도 부르지 않았으므로 working 이어야 합니다", id, status)
		}
	}

	// 필요해진 순간에 정확히 회수된다 — 죽은 것만.
	rescuer := f.AddWorktree("rescue", "br/rescue").Path
	f.RunAs(Identity{Agent: "rescuer", Session: "sr"}, rescuer, "next")
	for i, id := range ids {
		want := "working"
		if i == 1 {
			want = "open"
		}
		if status := f.show(id)["status"]; status != want {
			t.Fatalf("%s = %v, want %s", id, status, want)
		}
	}
}

// E2E-10: 세션 재시작.
//
// 잠금만 보고 session 을 비교하지 않는 구현은 실패한다. 새 프로세스가 잠금을
// 잡았으므로 "생존" 으로 보이지만, 이슈의 session 은 죽은 쪽이다 (§3.6).
func TestRestartedSessionLosesOldClaim(t *testing.T) {
	f := newFixture(t)
	dir := f.AddWorktree("b", "br/b").Path
	id := f.add("작업")

	first := f.AgentAs(Identity{Agent: "agent-b", Session: "s1"}, dir)
	if r := first.Run("claim", id); r.ExitCode != 0 {
		t.Fatalf("claim:\n%s", r)
	}
	first.Kill()

	// 같은 이름, 새 세션으로 재시작.
	second := f.AgentAs(Identity{Agent: "agent-b", Session: "s2"}, dir)
	if r := second.Run("list"); r.ExitCode != 0 {
		t.Fatalf("재시작한 세션의 명령:\n%s", r)
	}

	rescuer := f.AddWorktree("c", "br/c").Path
	f.RunAs(Identity{Agent: "agent-c", Session: "sc"}, rescuer, "next")

	if status := f.show(id)["status"]; status != "open" {
		t.Fatalf("status = %v — 잠금이 살아 있어도 죽은 세션의 claim 은 회수돼야 합니다", status)
	}
	// agent-b 자체는 살아 있다.
	found := false
	for _, lease := range f.leases() {
		if lease.Agent == "agent-b" {
			found = true
		}
	}
	if !found {
		t.Fatal("재시작한 agent-b 의 잠금 기록이 없습니다")
	}
}

// E2E-10b: worktree 배타 (암묵 세션).
//
// 초기화 명령 없이도 배타가 강제되어야 한다.
func TestWorktreeExclusion(t *testing.T) {
	f := newFixture(t)
	dir := f.AddWorktree("shared", "br/shared").Path
	first := Identity{Agent: "agent-a1", Session: "s1"}
	second := Identity{Agent: "agent-a2", Session: "s2"}
	id := f.add("작업")
	other := f.add("다른 작업")

	if r := f.RunAs(first, dir, "claim", id); r.ExitCode != 0 {
		t.Fatalf("첫 번째:\n%s", r)
	}
	r := f.RunAs(second, dir, "claim", other)
	if r.ExitCode == 0 {
		t.Fatal("두 번째 에이전트가 같은 worktree 에서 성공했습니다")
	}
	if !strings.Contains(r.Stderr, "in use by") || !strings.Contains(r.Stderr, "agent-a1") {
		t.Fatalf("오류 메시지가 누가 쥐고 있는지 밝히지 않습니다:\n%s", r)
	}
	// 첫 번째는 영향이 없다.
	if r := f.RunAs(first, dir, "start", id); r.ExitCode != 0 {
		t.Fatalf("첫 번째가 영향을 받았습니다:\n%s", r)
	}
	// 우회 경로는 열려 있다.
	if r := f.RunAs(second, dir, "claim", other, "--allow-shared-worktree"); r.ExitCode != 0 {
		t.Fatalf("--allow-shared-worktree:\n%s", r)
	}
}

// E2E-10c: 조회는 잠금과 무관.
//
// 조회가 잠금을 요구하면 한 worktree 에서 상태를 볼 수조차 없게 된다.
func TestQueriesIgnoreLocks(t *testing.T) {
	f := newFixture(t)
	dir := f.AddWorktree("a", "br/a").Path
	holder := Identity{Agent: "agent-a1", Session: "s1"}
	id := f.add("작업")
	if r := f.RunAs(holder, dir, "claim", id); r.ExitCode != 0 {
		t.Fatalf("claim:\n%s", r)
	}
	before := f.leases()

	intruder := Identity{Agent: "agent-a2", Session: "s2"}
	for _, args := range [][]string{
		{"list"}, {"show", id}, {"export"}, {"history", id}, {"agents"}, {"fsck"},
	} {
		if r := f.RunAs(intruder, dir, args...); r.ExitCode != 0 {
			t.Fatalf("%v 가 잠금 때문에 실패했습니다:\n%s", args, r)
		}
	}
	if !sameJSON(before, f.leases()) {
		t.Fatalf("조회가 잠금 기록을 바꿨습니다:\n%v\n%v", before, f.leases())
	}
}

// E2E-10d: 암묵 세션 자동 등록.
//
// 아무 초기화 없이 곧바로 next --claim 을 부른다.
func TestImplicitSessionRegistration(t *testing.T) {
	f := newFixture(t)
	f.add("작업")
	if len(f.leases()) != 0 {
		t.Fatalf("아직 아무것도 안 했는데 기록이 있습니다: %v", f.leases())
	}

	if r := f.Run("next", "--claim"); r.ExitCode != 0 {
		t.Fatalf("next --claim:\n%s", r)
	}
	leases := f.leases()
	if len(leases) != 1 || leases[0].Agent != "e2e-agent" {
		t.Fatalf("잠금 파일 = %v", leases)
	}
	agents, _ := f.RunJSON("agents").([]any)
	if len(agents) != 1 {
		t.Fatalf("agents = %v", agents)
	}
	if check := f.doctorCheck("worktree"); check["status"] != "OK" {
		t.Fatalf("doctor 의 worktree = %v", check)
	}
	// 이후 명령이 같은 세션으로 묶인다.
	if ids := f.listIDs("--mine"); len(ids) != 1 {
		t.Fatalf("--mine = %v, 같은 세션으로 묶이지 않았습니다", ids)
	}
}

// E2E-10e: 등록 경쟁.
//
// read-modify-write 가 원자적이지 않으면 실패한다 (§3.6).
func TestRegistrationRace(t *testing.T) {
	f := newFixture(t)
	dir := f.AddWorktree("shared", "br/shared").Path
	const n = 16
	for i := range n {
		f.add(fmt.Sprintf("작업 %02d", i))
	}

	results := race(f, n, func(i int, ident Identity) Result {
		return f.RunAs(ident, dir, "next", "--claim")
	})

	winners := 0
	for i, r := range results {
		if r.ExitCode == 0 {
			winners++
			continue
		}
		if !strings.Contains(r.Stderr, "in use by") {
			t.Fatalf("agent-%02d 의 거부 사유가 불명확합니다:\n%s", i, r)
		}
	}
	if winners != 1 {
		t.Fatalf("worktree 확보 %d건, want 1", winners)
	}
	// 잠금 파일이 손상되지 않았다. 읽히고, 승자와 일치한다.
	leases := f.leases()
	if len(leases) != 1 {
		t.Fatalf("잠금 기록 = %v", leases)
	}
	if leases[0].Worktree != dir {
		t.Fatalf("worktree = %q, want %q", leases[0].Worktree, dir)
	}
}

// E2E-10f: 세션 명령 부재.
//
// 세션 수명 동안 잠금을 쥐는 명령이 다시 들어오면 멈춘 프로세스가 worktree 를
// 영구히 붙잡는다 (§3.6).
func TestNoSessionCommands(t *testing.T) {
	f := newFixture(t)
	help := f.MustRun("--help").Stdout
	for _, banned := range []string{"session", "internal"} {
		for _, line := range lines(help) {
			if strings.HasPrefix(line, banned+" ") {
				t.Fatalf("--help 에 %q 명령이 노출됐습니다: %q", banned, line)
			}
		}
	}
	for _, sub := range []string{"begin", "end", "exec", "status"} {
		if r := f.Run("session", sub); r.ExitCode == 0 {
			t.Fatalf("session %s 가 동작합니다:\n%s", sub, r)
		}
	}
	// 초기화 없이 전체 워크플로우가 동작한다.
	id := f.add("작업")
	for _, step := range []string{"claim", "start", "done"} {
		if r := f.Run(step, id); r.ExitCode != 0 {
			t.Fatalf("%s:\n%s", step, r)
		}
	}
}

// E2E-10h: 장시간 작업 중 회수 방지.
//
// 배정 모델의 핵심 회귀다 (D11). 임계값을 짧게 바꾸면 산 작업이 회수된다.
func TestLongRunningWorkSurvivesThreshold(t *testing.T) {
	f := newFixture(t)
	worker := f.AddWorktree("b", "br/b").Path
	rescuer := f.AddWorktree("c", "br/c").Path
	id := f.add("긴 작업")

	// 훅 없음 — last_activity 경로다. 임계값을 줄여 시간을 흉내 낸다.
	// 8h 중 7h 경과는 임계값 8s 중 7s 와 같은 사건이다.
	b := Identity{Agent: "agent-b", Session: "sb"}
	f.Env = append(f.Env, "PPWK_ACTIVITY_TTL=30s")
	if r := f.RunAs(b, worker, "start", id); r.ExitCode != 0 {
		t.Fatalf("start:\n%s", r)
	}

	c := Identity{Agent: "agent-c", Session: "sc"}
	f.RunAs(c, rescuer, "next")
	shown := f.show(id)
	if shown["status"] != "working" || shown["owner"] != "agent-b" {
		t.Fatalf("임계값 이내인데 회수됐습니다: %v", shown)
	}

	// 임계값을 넘긴 시점을 흉내 낸다. 잠금 기록을 과거로 되돌린다.
	f.ageLease("agent-b", time.Hour)
	f.RunAs(c, rescuer, "next")
	if status := f.show(id)["status"]; status != "open" {
		t.Fatalf("임계값을 넘겼는데 회수되지 않았습니다: %v", status)
	}
}

// E2E-11: 쓰기 중 SIGKILL.
//
// 객체를 먼저 만들고 ref 를 나중에 바꾸므로 부분 상태가 없어야 한다 (§4.1).
func TestKillDuringWriteLeavesNoPartialState(t *testing.T) {
	f := newFixture(t)
	id := f.add("작업")
	ref := "refs/ppwk/issues/" + id

	// 시작 시점부터 여러 지점에서 끊어 본다. 어느 지점이든 결과는 둘 중
	// 하나여야 한다 — claim 되었거나, 되지 않았거나.
	for i := range 12 {
		delay := time.Duration(i) * 3 * time.Millisecond
		f.killDuring(delay, "claim", id)

		status := fmt.Sprint(f.show(id)["status"])
		switch status {
		case "open", "claimed":
		default:
			t.Fatalf("중간 상태 %q 로 남았습니다 (delay=%s)", status, delay)
		}
		if strings.TrimSpace(f.Git("cat-file", "-t", ref)) != "commit" {
			t.Fatalf("ref 가 commit 을 가리키지 않습니다 (delay=%s)", delay)
		}
		if status == "claimed" {
			break
		}
	}
	// dangling commit 은 허용된다. 정합성만 본다.
	f.MustRun("fsck")
	f.expectGitFsckClean()
}

// E2E-12: archive 이동 중 SIGKILL.
//
// 개별 update-ref 2회로 구현하면 실패한다 (§4.4).
func TestKillDuringArchiveKeepsExactlyOneRef(t *testing.T) {
	f := newFixture(t)
	for i := range 12 {
		id := f.add(fmt.Sprintf("작업 %02d", i))
		f.MustRun("start", id)
		f.killDuring(time.Duration(i)*3*time.Millisecond, "done", id)

		inIssues := f.HasRef("refs/ppwk/issues/" + id)
		inArchive := f.HasRef("refs/ppwk/archive/" + id)
		if inIssues && inArchive {
			t.Fatalf("%s 가 양쪽에 있습니다 — 트랜잭션이 아닙니다", id)
		}
		if !inIssues && !inArchive {
			t.Fatalf("%s 가 양쪽에서 사라졌습니다", id)
		}
	}
	f.expectGitFsckClean()
}

// E2E-13: stale lock.
//
// 도구가 남의 .lock 을 함부로 지우면, 진짜로 쓰는 중인 프로세스를 깨뜨린다.
func TestStaleLockIsReportedNotRemoved(t *testing.T) {
	f := newFixture(t)
	id := f.add("작업")
	ref := "refs/ppwk/issues/" + id
	lock := filepath.Join(f.commonDir(), ref+".lock")
	oid := strings.TrimSpace(f.Git("rev-parse", ref))
	writeFile(t, lock, oid+"\n")

	r := f.Run("claim", id)
	if r.ExitCode == 0 {
		t.Fatalf(".lock 이 있는데 claim 이 성공했습니다:\n%s", r)
	}
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("도구가 .lock 을 지웠습니다: %v", err)
	}

	if check := f.doctorCheck("stale locks"); check["status"] == "OK" {
		t.Fatalf("doctor 가 stale lock 을 보고하지 않습니다: %v", check)
	}
	findings := f.fsckChecks()
	if !findings["stale_lock"] {
		t.Fatalf("fsck 가 stale lock 을 보고하지 않습니다: %v", findings)
	}
}
