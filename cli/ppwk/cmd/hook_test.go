package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
)

// sessionEventCLI 는 stdin 을 주고 internal session-event 를 돌린다.
func sessionEventCLI(t *testing.T, dir, stdin string) error {
	t.Helper()
	root := New(Version{CLI: "test", Schema: "1"}, io.Discard, io.Discard)
	root.Reader = strings.NewReader(stdin)
	return root.Run(context.Background(), []string{"ppwk", "-C", dir, "internal", "session-event"})
}

// leaseOf 는 이 worktree 의 잠금 기록을 읽는다.
func leaseOf(t *testing.T, dir string) model.Lease {
	t.Helper()
	var payload struct {
		Data []model.Lease `json:"data"`
	}
	out := runCLI(t, dir, "agents", "--json")
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("agents --json: %v\n%s", err, out)
	}
	if len(payload.Data) != 1 {
		t.Fatalf("기록 %d개: %v", len(payload.Data), payload.Data)
	}
	return payload.Data[0]
}

// T11.1 SessionStart 훅이 세션을 등록하고 hook_pid 를 기록한다.
//
// T11.3 도 함께 본다 — session_id 와 cwd 가 실제로 쓰인다.
func TestSessionStartRecordsHookPID(t *testing.T) {
	dir := doctorRepo(t)
	payload := `{"session_id":"conv-abc","cwd":"` + dir + `","hook_event_name":"SessionStart"}`

	// -C 는 다른 곳을 가리키게 두어 cwd 가 쓰이는지 본다.
	other := t.TempDir()
	if err := sessionEventCLI(t, other, payload); err != nil {
		t.Fatal(err)
	}

	lease := leaseOf(t, dir)
	if lease.Session != "conv-abc" {
		t.Fatalf("session = %q — stdin 의 session_id 가 쓰이지 않았습니다", lease.Session)
	}
	if lease.HookPID == nil || *lease.HookPID != os.Getppid() {
		t.Fatalf("hook_pid = %v, want %d", lease.HookPID, os.Getppid())
	}
	if lease.HookStarttime == nil || *lease.HookStarttime == "" {
		t.Fatalf("hook_starttime = %v — pid 재사용을 거를 수 없습니다", lease.HookStarttime)
	}
}

// hook_pid 는 이후 평범한 명령이 지우지 않는다.
//
// 지우면 첫 claim 한 번으로 즉시 감지가 8시간 임계값으로 되돌아간다.
func TestOrdinaryCommandKeepsHookPID(t *testing.T) {
	dir := doctorRepo(t)
	payload := `{"session_id":"conv-abc","cwd":"` + dir + `","hook_event_name":"SessionStart"}`
	if err := sessionEventCLI(t, dir, payload); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PPWK_SESSION", "conv-abc")
	id := strings.TrimSpace(runCLI(t, dir, "add", "대상"))
	runCLI(t, dir, "claim", id)

	if lease := leaseOf(t, dir); lease.HookPID == nil {
		t.Fatal("claim 이 hook_pid 를 지웠습니다")
	}
}

// T11.2 / T11.2b SessionEnd 는 claimed 만 반납하고 working 은 그대로 둔다.
//
// working 에는 worktree 의 미커밋 변경이 있을 수 있고, 사용자가 도구를 닫았다
// 다시 열어 잇는 것은 흔하다 (D15).
func TestSessionEndReleasesClaimedOnly(t *testing.T) {
	dir := doctorRepo(t)
	t.Setenv("PPWK_SESSION", "conv-abc")

	claimed := strings.TrimSpace(runCLI(t, dir, "add", "예약만"))
	working := strings.TrimSpace(runCLI(t, dir, "add", "작업 중"))
	runCLI(t, dir, "claim", claimed)
	runCLI(t, dir, "start", working)

	payload := `{"session_id":"conv-abc","cwd":"` + dir + `","hook_event_name":"SessionEnd"}`
	if err := sessionEventCLI(t, dir, payload); err != nil {
		t.Fatal(err)
	}

	if out := runCLI(t, dir, "show", claimed); !strings.Contains(out, "open") {
		t.Fatalf("claimed 가 반납되지 않았습니다:\n%s", out)
	}
	if out := runCLI(t, dir, "show", working); !strings.Contains(out, "working") {
		t.Fatalf("working 을 건드렸습니다 (D15):\n%s", out)
	}
}

// T11.4 / T11.5 알 수 없는 입력과 빈 입력은 조용히 exit 0 한다.
//
// 훅이 실패하면 도구 세션 자체가 막히거나 지연된다. 정합성은 층 1 이
// 단독으로 보장하므로 이 층은 실패해도 안전하다.
func TestSessionEventAlwaysExitsZero(t *testing.T) {
	dir := doctorRepo(t)
	for _, tc := range []struct{ name, stdin string }{
		{"빈 입력", ""},
		{"JSON 아님", "이건 JSON 이 아니다"},
		{"모르는 이벤트", `{"hook_event_name":"SubagentStart","cwd":"` + dir + `"}`},
		{"필드 없음", `{}`},
		{"저장소 밖", `{"hook_event_name":"SessionStart","cwd":"/nonexistent-path-xyz"}`},
		{"스키마가 바뀜", `{"hook_event_name":"SessionStart","cwd":"` + dir + `","새 필드":123}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := sessionEventCLI(t, dir, tc.stdin); err != nil {
				t.Fatalf("exit != 0: %v", err)
			}
		})
	}
}

// SessionEnd 가 두 번 와도 멱등하고, SessionStart 없이 와도 무해하다.
func TestSessionEndIsIdempotent(t *testing.T) {
	dir := doctorRepo(t)
	t.Setenv("PPWK_SESSION", "conv-abc")
	id := strings.TrimSpace(runCLI(t, dir, "add", "대상"))
	runCLI(t, dir, "claim", id)

	payload := `{"session_id":"conv-abc","cwd":"` + dir + `","hook_event_name":"SessionEnd"}`
	for range 2 {
		if err := sessionEventCLI(t, dir, payload); err != nil {
			t.Fatal(err)
		}
	}
	if out := runCLI(t, dir, "show", id); !strings.Contains(out, "open") {
		t.Fatalf("show:\n%s", out)
	}
}

// T11.6 SessionStart 는 ref 를 하나도 쓰지 않는다.
//
// 명세의 상한은 "ref 쓰기 1회" 지만, lease ref 를 없앤 뒤로(D13) 세션 등록은
// 잠금 파일만 건드린다. 훅은 도구 세션 안에서 동기 실행되므로 이 성질이
// 곧 속도다.
func TestSessionStartWritesNoRefs(t *testing.T) {
	dir := doctorRepo(t)
	before := refSnapshot(t, dir)

	payload := `{"session_id":"conv-abc","cwd":"` + dir + `","hook_event_name":"SessionStart"}`
	if err := sessionEventCLI(t, dir, payload); err != nil {
		t.Fatal(err)
	}
	if after := refSnapshot(t, dir); after != before {
		t.Fatalf("ref 가 바뀌었습니다:\n%s\n→\n%s", before, after)
	}
}

// refSnapshot 은 refs/ppwk/ 전체를 한 문자열로 만든다.
func refSnapshot(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname) %(objectname)", "refs/ppwk/")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// T11.7 / T11.8 훅이 없어도 전체 흐름이 정상이고, 회수는 잠금 확인이 한다.
//
// 훅은 최적화이지 정합성의 근거가 아니다. SessionEnd 가 영영 오지 않아도
// 층 1 이 처리해야 한다.
func TestWorkflowWithoutHooks(t *testing.T) {
	dir := doctorRepo(t)
	t.Setenv("PPWK_SESSION", "dead-session")
	id := strings.TrimSpace(runCLI(t, dir, "add", "대상"))
	runCLI(t, dir, "claim", id)

	// 훅은 설치되지 않았고 SessionEnd 도 오지 않았다.
	if status := hookStatusJSON(t, dir); status["claude-code"].Configured {
		t.Fatal("훅이 설치돼 있습니다")
	}

	// 임계값을 넘기면 다음 next 가 회수한다.
	t.Setenv("PPWK_ACTIVITY_TTL", "1ns")
	t.Setenv("PPWK_SESSION", "live-session")
	if got := strings.TrimSpace(runCLI(t, dir, "next", "--claim")); got != id {
		t.Fatalf("next = %q, want %s — 잠금 확인만으로 회수되어야 합니다", got, id)
	}
}

type hookStatus struct {
	Tool       string          `json:"tool"`
	Path       string          `json:"path"`
	Configured bool            `json:"configured"`
	Events     map[string]bool `json:"events"`
	Installed  bool            `json:"installed"`
}

func hookStatusJSON(t *testing.T, dir string) map[string]hookStatus {
	t.Helper()
	var payload struct {
		Data []hookStatus `json:"data"`
	}
	out := runCLI(t, dir, "hook", "status", "--json")
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("hook status --json: %v\n%s", err, out)
	}
	byName := map[string]hookStatus{}
	for _, status := range payload.Data {
		byName[status.Tool] = status
	}
	return byName
}

// hook install / uninstall / status 가 CLI 로 동작한다.
func TestHookInstallLifecycle(t *testing.T) {
	dir := doctorRepo(t)

	// 대상을 고르지 않으면 사용법 오류다.
	if _, err := runCLIErr(t, dir, "hook", "install"); exitCode(t, err) != ExitUsage {
		t.Fatalf("exit %d, want %d", exitCode(t, err), ExitUsage)
	}

	runCLI(t, dir, "hook", "install", "--agent-tools")
	status := hookStatusJSON(t, dir)
	for _, tool := range []string{"claude-code", "codex"} {
		if !status[tool].Configured || !status[tool].Events["SessionStart"] ||
			!status[tool].Events["SessionEnd"] {
			t.Fatalf("%s = %+v", tool, status[tool])
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "settings.json")); err != nil {
		t.Fatal(err)
	}

	// 사람이 읽는 출력도 이벤트를 구분해 보여준다.
	if out := runCLI(t, dir, "hook", "status"); !strings.Contains(out, "SessionStart ✓") {
		t.Fatalf("status:\n%s", out)
	}

	runCLI(t, dir, "hook", "uninstall", "--agent-tools")
	if hookStatusJSON(t, dir)["claude-code"].Installed {
		t.Fatal("제거되지 않았습니다")
	}
}

// T10.1 git 의 reference-transaction 훅은 두지 않는다.
//
// polling 이 기본이고 hook 은 최적화라는 §6.1 의 결론을 따른 것이다. 알림
// 지연 1~2초를 없애는 대가로 socat 의존, 공용 hooks 디렉터리 설치, socket
// 수명 관리, 그리고 잘못하면 저장소의 모든 ref 쓰기가 멈추는 실패 모드를
// 함께 사 와야 한다. 되살리고 싶어지는 종류라 회귀로 못박는다.
func TestGitHookSurfaceAbsent(t *testing.T) {
	var stdout bytes.Buffer
	root := New(Version{CLI: "test", Schema: "1"}, &stdout, io.Discard)
	if err := root.Run(context.Background(), []string{"ppwk", "hook", "install", "--help"}); err != nil {
		t.Fatal(err)
	}
	if err := root.Run(context.Background(), []string{"ppwk", "init", "--help"}); err != nil {
		t.Fatal(err)
	}
	if err := root.Run(context.Background(), []string{"ppwk", "watch", "--help"}); err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"--git", "reference-transaction", "socket", "--hooks", "--hook "} {
		if strings.Contains(stdout.String(), banned) {
			t.Fatalf("%q 가 도움말에 있습니다 — git 훅 경로는 채택하지 않았습니다:\n%s",
				banned, stdout.String())
		}
	}
}
