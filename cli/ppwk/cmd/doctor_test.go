package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
)

func doctorRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main", "."},
		{"config", "user.name", "test"},
		{"config", "user.email", "test@example.com"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	runCLI(t, dir, "init")
	return dir
}

// runCLI 는 명령 하나를 실행하고 stdout 을 돌려준다.
func runCLI(t *testing.T, dir string, args ...string) string {
	t.Helper()
	var stdout bytes.Buffer
	root := New(Version{CLI: "test", Schema: "1"}, &stdout, io.Discard)
	full := append([]string{"ppwk", "-C", dir}, args...)
	if err := root.Run(context.Background(), full); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return stdout.String()
}

func doctorChecksJSON(t *testing.T, dir string) map[string]check {
	t.Helper()
	var payload struct {
		Data struct {
			Checks []check `json:"checks"`
		} `json:"data"`
	}
	out := runCLI(t, dir, "doctor", "--json")
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, out)
	}
	byName := make(map[string]check, len(payload.Data.Checks))
	for _, c := range payload.Data.Checks {
		byName[c.Name] = c
	}
	return byName
}

// T4.27 — doctor 가 감지 근거를 환경변수 이름으로 표시한다.
//
// --agent 플래그에 Sources: cli.EnvVars("PPWK_AGENT") 를 달면 환경변수로 온
// 값이 플래그로 온 값과 구분되지 않아 근거가 "--agent" 로 뭉개진다. 그 한 줄을
// 되돌리면 이 테스트가 깨져야 한다.
func TestDoctorReportsEnvProvenance(t *testing.T) {
	dir := doctorRepo(t)
	t.Setenv("PPWK_AGENT", "orchestrated-3")
	t.Setenv("PPWK_SESSION", "sess-9")

	checks := doctorChecksJSON(t, dir)
	agent := checks["agent id"]
	if agent.Value != "orchestrated-3" || agent.Via != "PPWK_AGENT" {
		t.Fatalf("agent id = %q via %q, want orchestrated-3 via PPWK_AGENT", agent.Value, agent.Via)
	}
	if session := checks["session id"]; session.Via != "PPWK_SESSION" {
		t.Fatalf("session id via %q, want PPWK_SESSION", session.Via)
	}
}

// --agent 로 준 값은 플래그로 보고된다.
func TestDoctorReportsFlagProvenance(t *testing.T) {
	dir := doctorRepo(t)
	t.Setenv("PPWK_AGENT", "from-env")

	var stdout bytes.Buffer
	root := New(Version{CLI: "test", Schema: "1"}, &stdout, io.Discard)
	if err := root.Run(context.Background(), []string{
		"ppwk", "-C", dir, "--agent", "from-flag", "doctor", "--json",
	}); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Data struct {
			Checks []check `json:"checks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, c := range payload.Data.Checks {
		if c.Name != "agent id" {
			continue
		}
		if c.Value != "from-flag" || c.Via != "--agent" {
			t.Fatalf("agent id = %q via %q", c.Value, c.Via)
		}
		return
	}
	t.Fatal("agent id 항목이 없다")
}

// doctor 는 항목마다 상태를 갖고, --json 에도 그 상태가 실린다 (features §1).
//
// 사람이 보는 출력에만 WARN 이 있으면 --json 소비자는 경고를 볼 수 없다.
func TestDoctorReportsStatusPerCheck(t *testing.T) {
	dir := doctorRepo(t)
	t.Setenv("PPWK_AGENT", "solo")
	t.Setenv("PPWK_SESSION", "sess-1")

	checks := doctorChecksJSON(t, dir)
	for _, name := range []string{"agent id", "session id", "file locking", "worktree", "liveness", "holding"} {
		c, ok := checks[name]
		if !ok {
			t.Fatalf("%q 항목이 없다", name)
		}
		switch c.Status {
		case statusOK, statusWarn, statusFail:
		default:
			t.Fatalf("%q status=%q", name, c.Status)
		}
	}
	// 로컬 임시 디렉터리이므로 flock 은 동작해야 한다.
	if got := checks["file locking"].Status; got != statusOK {
		t.Fatalf("file locking = %s", got)
	}
	// 훅이 없으므로 liveness 는 WARN 이고 근거가 붙는다.
	if got := checks["liveness"]; got.Status != statusWarn || got.Hint == "" {
		t.Fatalf("liveness = %s hint=%q", got.Status, got.Hint)
	}
}

// doctor 는 조회 명령이다 — 세션을 등록하지 않는다 (T4.16d, §3.6).
func TestDoctorDoesNotRegisterSession(t *testing.T) {
	dir := doctorRepo(t)
	t.Setenv("PPWK_AGENT", "solo")
	t.Setenv("PPWK_SESSION", "sess-1")

	if got := doctorChecksJSON(t, dir)["worktree"].Via; !strings.Contains(got, "미등록") {
		t.Fatalf("worktree via=%q — doctor 가 세션을 등록했다", got)
	}
	if got := runCLI(t, dir, "agents"); strings.TrimSpace(got) != "" {
		t.Fatalf("agents = %q — 조회 명령이 잠금을 남겼다", got)
	}
}

// 보유 이슈와 worktree 배타 확보가 상태 변경 뒤에 반영된다.
func TestDoctorReportsHoldingAfterClaim(t *testing.T) {
	dir := doctorRepo(t)
	t.Setenv("PPWK_AGENT", "solo")
	t.Setenv("PPWK_SESSION", "sess-1")
	runCLI(t, dir, "add", "제목")
	runCLI(t, dir, "claim", "T001")

	checks := doctorChecksJSON(t, dir)
	if got := checks["holding"].Value; got != "T001" {
		t.Fatalf("holding=%q, want T001", got)
	}
	if got := checks["worktree"]; got.Status != statusOK || !strings.Contains(got.Via, "배타 확보") {
		t.Fatalf("worktree = %s via %q", got.Status, got.Via)
	}
}
