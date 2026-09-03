package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// E2E-22b: 환경변수 감지 (훅 없이).
//
// 도구 훅은 선택 기능이므로, 없을 때도 전부 동작해야 한다.
func TestEnvironmentDetectionWithoutHooks(t *testing.T) {
	f := newFixture(t)
	// 픽스처의 고정 신원을 걷어내고 도구 환경만 남긴다.
	f.Env = []string{"CLAUDECODE=1", "CLAUDE_CODE_SESSION_ID=session-abc"}

	agent := f.agentID()
	if !strings.HasPrefix(agent, "claude-code:") {
		t.Fatalf("agent id = %q, want claude-code:<worktree>", agent)
	}
	if !strings.HasSuffix(agent, filepath.Base(f.Root)) {
		t.Fatalf("agent id = %q, worktree 이름으로 끝나야 합니다", agent)
	}
	if check := f.doctorCheck("session id"); check["value"] != "session-abc" ||
		!strings.Contains(strings.ToLower(check["via"].(string)), "claude_code_session_id") {
		t.Fatalf("doctor 가 감지 근거를 밝히지 않습니다: %v", check)
	}

	// 전체 워크플로우가 동작하고, 같은 세션으로 묶인다.
	mine := f.add("내 작업")
	f.MustRun("claim", mine)
	if ids := f.listIDs("--mine"); len(ids) != 1 || ids[0] != mine {
		t.Fatalf("--mine = %v, want [%s]", ids, mine)
	}
	// 다른 세션의 것은 --mine 에 들어오지 않는다.
	other := f.AddWorktree("other", "br/other")
	theirs := issueID(t, f.RunJSONIn(other, "add", "남의 작업"))
	f.exec(other.Path, []string{"CLAUDE_CODE_SESSION_ID=session-xyz"}, "claim", theirs)
	if ids := f.listIDs("--mine"); len(ids) != 1 || ids[0] != mine {
		t.Fatalf("--mine = %v, 남의 세션 것이 섞였습니다", ids)
	}
	f.MustRun("start", mine)
	f.MustRun("done", mine)
}

// E2E-22c: 감지 실패 폴백.
//
// 환경변수 이름은 도구 버전에 따라 바뀔 수 있다. 이 시나리오가 그 변화에
// 대한 내성을 검증한다.
func TestFallbackWhenDetectionFails(t *testing.T) {
	f := newFixture(t)
	f.Env = nil // 아무 힌트도 없는 환경

	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	want := host + ":" + filepath.Base(f.Root)
	if agent := f.agentID(); agent != want {
		t.Fatalf("agent id = %q, want %q", agent, want)
	}
	// 폴백은 정보다. FAIL 이 아니다.
	if check := f.doctorCheck("agent id"); check["status"] != "OK" {
		t.Fatalf("폴백이 FAIL 로 보고됐습니다: %v", check)
	}
	if r := f.Run("doctor"); r.ExitCode != 0 {
		t.Fatalf("doctor 가 실패했습니다:\n%s", r)
	}

	// 모든 기능이 정상 동작한다.
	id := f.add("작업")
	for _, step := range []string{"claim", "start", "done"} {
		if r := f.Run(step, id); r.ExitCode != 0 {
			t.Fatalf("%s:\n%s", step, r)
		}
	}
}

// E2E-22d: SessionEnd 정상 경로.
//
// 대기 시간 없이 반납된다. 이것이 훅을 두는 유일한 이유다 — 정합성이 아니라
// 속도다 (§3.8).
func TestSessionEndReleasesClaims(t *testing.T) {
	f := newFixture(t)
	dir := f.AddWorktree("b", "br/b").Path
	var ids []string
	for i := range 3 {
		ids = append(ids, f.add("작업 "+string(rune('a'+i))))
	}

	agent := f.Agent("agent-b", dir)
	for _, id := range ids {
		if r := agent.Run("claim", id); r.ExitCode != 0 {
			t.Fatalf("claim:\n%s", r)
		}
	}
	agent.SessionEnd()

	for _, id := range ids {
		shown := f.show(id)
		if shown["status"] != "open" || shown["owner"] != nil {
			t.Fatalf("%s = %v, SessionEnd 시점에 반납돼야 합니다", id, shown)
		}
	}
	// 다른 에이전트가 곧바로 가져간다 — 회수를 기다리지 않는다.
	other := f.AddWorktree("c", "br/c").Path
	if r := f.RunAs(Identity{Agent: "agent-c", Session: "sc"}, other, "claim", ids[0]); r.ExitCode != 0 {
		t.Fatalf("즉시 재배정:\n%s", r)
	}
}

// E2E-22e: SessionEnd 누락 시 폴백.
//
// 훅이 정합성의 근거가 아님을 검증한다. 층 1 단독으로 처리되어야 한다.
func TestMissingSessionEndFallsBackToLocks(t *testing.T) {
	f := newFixture(t)
	dir := f.AddWorktree("b", "br/b").Path
	id := f.add("작업")

	agent := f.Agent("agent-b", dir)
	if r := agent.Run("claim", id); r.ExitCode != 0 {
		t.Fatalf("claim:\n%s", r)
	}
	// SessionEnd 를 건너뛰고 죽는다.
	agent.Kill()

	other := f.AddWorktree("c", "br/c").Path
	f.RunAs(Identity{Agent: "agent-c", Session: "sc"}, other, "next")
	if status := f.show(id)["status"]; status != "open" {
		t.Fatalf("status = %v, want open — 훅이 없어도 층 1 이 처리해야 합니다", status)
	}
}

// E2E-22f: 훅이 세션을 막지 않음.
//
// 훅은 도구의 시작 경로에 있다. 여기서 실패하거나 오래 걸리면 ppwk 와 무관한
// 작업까지 막힌다. 그래서 무슨 입력이 와도 조용히 성공한다.
func TestHookNeverBlocksSession(t *testing.T) {
	f := newFixture(t)
	outside := t.TempDir()

	for _, tc := range []struct {
		name  string
		stdin string
		dir   string
	}{
		{"빈 입력", "", f.Root},
		{"JSON 아님", "not json at all", f.Root},
		{"빈 객체", "{}", f.Root},
		{"모르는 이벤트", `{"hook_event_name":"SomethingElse"}`, f.Root},
		{"저장소 밖 cwd", `{"hook_event_name":"SessionStart","cwd":"` + outside + `"}`, outside},
		{"초기화 안 된 저장소", `{"hook_event_name":"SessionStart"}`, outside},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(binary, "internal", "session-event")
			cmd.Dir = tc.dir
			cmd.Env = baseEnv()
			cmd.Stdin = strings.NewReader(tc.stdin)
			start := time.Now()
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("exit 0 이어야 합니다: %v\n%s", err, out)
			}
			// 도구의 시작을 눈에 띄게 늦추지 않는다.
			if elapsed := time.Since(start); elapsed > 3*time.Second {
				t.Fatalf("훅이 %s 걸렸습니다", elapsed)
			}
		})
	}
}

// E2E-22g: SessionEnd 가 working 을 보존.
//
// claimed 는 아직 시작하지 않은 예약이므로 반납해도 잃을 것이 없다. working
// 은 다르다 — 커밋하지 않은 작업이 있을 수 있다 (D15).
func TestSessionEndPreservesWorking(t *testing.T) {
	f := newFixture(t)
	dir := f.AddWorktree("b", "br/b").Path
	working := f.add("진행 중")
	claimed := f.add("예약만")

	agent := f.Agent("agent-b", dir)
	if r := agent.Run("start", working); r.ExitCode != 0 {
		t.Fatalf("start:\n%s", r)
	}
	if r := agent.Run("claim", claimed); r.ExitCode != 0 {
		t.Fatalf("claim:\n%s", r)
	}
	agent.SessionEnd()

	if got := f.show(claimed); got["status"] != "open" {
		t.Fatalf("claimed = %v, want open", got["status"])
	}
	got := f.show(working)
	if got["status"] != "working" || got["owner"] != "agent-b" {
		t.Fatalf("working = %v, 미커밋 작업 보호를 위해 유지돼야 합니다", got)
	}
}

// E2E-10g: 생존 판정 2수준.
//
// 감지 속도는 통합 수준에 비례하되, 정합성은 두 구성에서 동일해야 한다 (§3.6).
func TestLivenessLevelsAgreeOnOutcome(t *testing.T) {
	t.Run("훅 없음", func(t *testing.T) {
		f := newFixture(t)
		f.Env = append(f.Env, "PPWK_ACTIVITY_TTL=30s")
		worker := f.AddWorktree("b", "br/b").Path
		id := f.add("작업")
		b := Identity{Agent: "agent-b", Session: "sb"}
		if r := f.RunAs(b, worker, "claim", id); r.ExitCode != 0 {
			t.Fatalf("claim:\n%s", r)
		}
		if check := f.doctorCheckIn(worker, b.env(), "liveness"); check["via"] != "훅 없음" {
			t.Fatalf("판정 근거 = %v, want 훅 없음", check)
		}

		rescuer := f.AddWorktree("c", "br/c").Path
		c := Identity{Agent: "agent-c", Session: "sc"}
		// 임계값 이내에는 회수되지 않는다.
		f.RunAs(c, rescuer, "next")
		if status := f.show(id)["status"]; status != "claimed" {
			t.Fatalf("임계값 이내인데 회수됐습니다: %v", status)
		}
		// 임계값을 넘기면 회수된다.
		f.ageLease("agent-b", time.Hour)
		f.RunAs(c, rescuer, "next")
		if status := f.show(id)["status"]; status != "open" {
			t.Fatalf("status = %v, want open", status)
		}
	})

	t.Run("훅 설치", func(t *testing.T) {
		f := newFixture(t)
		worker := f.AddWorktree("b", "br/b").Path
		id := f.add("작업")
		agent := f.Agent("agent-b", worker)
		if r := agent.Run("claim", id); r.ExitCode != 0 {
			t.Fatalf("claim:\n%s", r)
		}
		check := f.doctorCheckIn(worker, agent.Identity.env(), "liveness")
		if check["via"] != "즉시 감지" || !strings.HasPrefix(check["value"].(string), "hook_pid ") {
			t.Fatalf("판정 근거 = %v, want hook_pid / 즉시 감지", check)
		}

		// 임계값을 기다리지 않는다. 죽는 즉시 회수된다.
		agent.Kill()
		rescuer := f.AddWorktree("c", "br/c").Path
		f.RunAs(Identity{Agent: "agent-c", Session: "sc"}, rescuer, "next")
		if status := f.show(id)["status"]; status != "open" {
			t.Fatalf("status = %v, want open (즉시 회수)", status)
		}
	})
}

// E2E-27: 도구 훅 설정 병합.
//
// 설정 파일은 사람이 쓰는 것이다. 우리가 아는 키만 손대고 나머지는 바이트
// 그대로 둔다.
func TestHookInstallMergesConfig(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(f.Root, ".claude", "settings.json")
	original := `{
  "hooks": {
    "SessionStart": [
      {"hooks": [{"type": "command", "command": "echo 남의 훅"}]}
    ]
  },
  "unknownKey": {"nested": [1, 2, 3]},
  "model": "opus"
}`
	writeFile(t, path, original)

	f.MustRun("hook", "install", "--agent-tools")
	f.MustRun("hook", "install", "--agent-tools") // 두 번 설치해도 늘지 않는다

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("설정이 JSON 이 아니게 됐습니다: %v\n%s", err, raw)
	}
	// 모르는 키가 보존된다.
	if config["model"] != "opus" {
		t.Fatalf("model 이 사라졌습니다:\n%s", raw)
	}
	if _, ok := config["unknownKey"]; !ok {
		t.Fatalf("unknownKey 가 사라졌습니다:\n%s", raw)
	}
	// 남의 훅이 남아 있고, 우리 훅이 한 번만 들어간다.
	body := string(raw)
	if !strings.Contains(body, "echo 남의 훅") {
		t.Fatalf("남의 훅이 사라졌습니다:\n%s", body)
	}
	if n := strings.Count(body, "ppwk internal session-event"); n != 2 {
		t.Fatalf("ppwk 훅이 %d개, want 2 (SessionStart/SessionEnd):\n%s", n, body)
	}
	// Subagent 계열에는 등록하지 않는다.
	for _, banned := range []string{"SubagentStart", "SubagentStop"} {
		if strings.Contains(body, banned) {
			t.Fatalf("%s 에 등록됐습니다:\n%s", banned, body)
		}
	}

	// status 가 설치를 인정한다.
	f.expectHookStatus("claude-code", true)
	f.MustRun("hook", "uninstall", "--agent-tools")
	f.expectHookStatus("claude-code", false)
	after, _ := os.ReadFile(path)
	if !strings.Contains(string(after), "echo 남의 훅") {
		t.Fatalf("uninstall 이 남의 훅을 지웠습니다:\n%s", after)
	}
}
