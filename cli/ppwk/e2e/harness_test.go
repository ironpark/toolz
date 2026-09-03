// Package e2e 는 빌드된 ppwk 바이너리를 실제 저장소에 대고 돌린다.
//
// 이 패키지의 테스트는 Go 함수를 직접 부르지 않는다 (e2e 명세 §0.1). 종료
// 코드·출력 형식·플래그 파싱·프로세스 경계는 함수 호출로는 검증되지 않고,
// 이 시스템의 버그는 대부분 그 경계에서 나온다.
package e2e

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// binary 는 TestMain 이 한 번 빌드한 ppwk 실행 파일이다.
var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ppwk-e2e-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "ppwk")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "빌드 실패: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// baseEnv 는 통제된 환경을 처음부터 짓는다.
//
// 지울 목록을 적는 방식은 쓸 수 없다. 도구 감지는 수백 개의 환경변수를 보고,
// 그 목록은 라이브러리가 갱신될 때마다 늘어난다. 하나라도 새면 agent-id 가
// 테스트가 정한 값이 아니라 이 테스트를 돌리는 도구의 값이 된다. 그래서
// 남길 것만 적는다.
func baseEnv() []string {
	env := []string{
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=e2e", "GIT_AUTHOR_EMAIL=e2e@example.invalid",
		"GIT_COMMITTER_NAME=e2e", "GIT_COMMITTER_EMAIL=e2e@example.invalid",
		"NO_COLOR=1", "LC_ALL=C",
	}
	// git 과 sh 를 찾으려면 이 정도는 있어야 한다.
	for _, keep := range []string{"PATH", "HOME", "TMPDIR", "SHELL", "USER"} {
		if v, ok := os.LookupEnv(keep); ok {
			env = append(env, keep+"="+v)
		}
	}
	return env
}

// Result 는 한 번의 실행 결과다.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func (r Result) String() string {
	return fmt.Sprintf("exit=%d\n--- stdout\n%s--- stderr\n%s", r.ExitCode, r.Stdout, r.Stderr)
}

// Fixture 는 초기화된 저장소 하나와 그 위의 worktree 들이다.
type Fixture struct {
	t    *testing.T
	Root string
	// Base 는 init 과 그 산출물을 커밋한 시점이다. 오염 검사(§7)의 기준선이다.
	Base string
	// Env 는 이 픽스처의 모든 실행에 붙는 추가 환경변수다.
	Env []string

	mu        sync.Mutex
	worktrees []*Worktree
}

// Worktree 는 linked worktree 하나다.
type Worktree struct {
	Name string
	Path string
}

// newFixture 는 초기 commit 1개를 가진 저장소에 ppwk init 까지 마친다.
//
// 빈 저장소는 HEAD 가 없어 동작이 다르므로 반드시 commit 을 하나 만든다.
func newFixture(t *testing.T, gitInitArgs ...string) *Fixture {
	t.Helper()
	f := &Fixture{t: t, Root: tempRepo(t)}
	// 기본 신원을 고정한다. 그러지 않으면 실행마다 세션 nonce 가 새로 생겨
	// 같은 worktree 를 두 번째 명령이 쥐지 못한다 — 실제 도구는 세션 하나에
	// 여러 명령을 실행하므로, 그쪽이 기본값이어야 한다.
	f.Env = []string{"PPWK_AGENT=e2e-agent", "PPWK_SESSION=e2e-session"}

	args := append([]string{"init", "--quiet", "--initial-branch=main"}, gitInitArgs...)
	f.gitIn(f.Root, append(args, ".")...)
	f.gitIn(f.Root, "config", "user.name", "e2e")
	f.gitIn(f.Root, "config", "user.email", "e2e@example.invalid")
	writeFile(t, filepath.Join(f.Root, "README.md"), "e2e\n")
	f.gitIn(f.Root, "add", "README.md")
	f.gitIn(f.Root, "commit", "--quiet", "-m", "initial")

	if r := f.Run("init"); r.ExitCode != 0 {
		t.Fatalf("ppwk init:\n%s", r)
	}
	// init 이 만든 문서를 커밋해 베이스라인을 잡는다. E2E-23 이 그 뒤의
	// 워킹 디렉터리 변화를 전부 오염으로 본다.
	f.gitIn(f.Root, "add", "-A")
	f.gitIn(f.Root, "commit", "--quiet", "-m", "ppwk init")
	f.Base = strings.TrimSpace(f.Git("rev-parse", "HEAD"))
	return f
}

// tempRepo 는 심볼릭 링크가 풀린 임시 디렉터리다.
//
// macOS 의 /var 는 /private/var 의 링크다. 링크를 남겨 두면 worktree 경로
// 비교가 실패하므로 여기서 한 번 정규화한다 (§1.2).
func tempRepo(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// AddWorktree 는 새 브랜치의 linked worktree 를 만든다.
func (f *Fixture) AddWorktree(name, branch string) *Worktree {
	f.t.Helper()
	path := filepath.Join(filepath.Dir(f.Root), "wt-"+name)
	f.gitIn(f.Root, "worktree", "add", "--quiet", "-b", branch, path)
	wt := &Worktree{Name: name, Path: path}
	f.mu.Lock()
	f.worktrees = append(f.worktrees, wt)
	f.mu.Unlock()
	return wt
}

// Run 은 저장소 최상단에서 ppwk 를 실행한다.
func (f *Fixture) Run(args ...string) Result {
	f.t.Helper()
	return f.exec(f.Root, nil, args...)
}

// RunIn 은 특정 worktree 에서 실행한다.
func (f *Fixture) RunIn(wt *Worktree, args ...string) Result {
	f.t.Helper()
	return f.exec(wt.Path, nil, args...)
}

// RunAs 는 신원을 지정해 실행한다. 여러 에이전트를 흉내 내는 기본 수단이다.
func (f *Fixture) RunAs(id Identity, dir string, args ...string) Result {
	f.t.Helper()
	return f.exec(dir, id.env(), args...)
}

// Identity 는 한 에이전트의 신원이다.
type Identity struct {
	Agent   string
	Session string
}

func (i Identity) env() []string {
	var env []string
	if i.Agent != "" {
		env = append(env, "PPWK_AGENT="+i.Agent)
	}
	if i.Session != "" {
		env = append(env, "PPWK_SESSION="+i.Session)
	}
	return env
}

func (f *Fixture) exec(dir string, extra []string, args ...string) Result {
	f.t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = append(append(baseEnv(), f.Env...), extra...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !asExitError(err, &exit) {
			f.t.Fatalf("ppwk %s: %v", strings.Join(args, " "), err)
		}
		code = exit.ExitCode()
	}
	return Result{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: code}
}

func asExitError(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*target = e
	}
	return ok
}

// MustRun 은 실패하면 테스트를 끝낸다.
func (f *Fixture) MustRun(args ...string) Result {
	f.t.Helper()
	r := f.Run(args...)
	if r.ExitCode != 0 {
		f.t.Fatalf("ppwk %s 실패:\n%s", strings.Join(args, " "), r)
	}
	return r
}

// RunJSON 은 --json 출력을 파싱한다. envelope 의 data 만 돌려준다.
func (f *Fixture) RunJSON(args ...string) any {
	f.t.Helper()
	return f.runJSONIn(f.Root, nil, args...)
}

func (f *Fixture) RunJSONIn(wt *Worktree, args ...string) any {
	f.t.Helper()
	return f.runJSONIn(wt.Path, nil, args...)
}

func (f *Fixture) RunJSONAs(id Identity, dir string, args ...string) any {
	f.t.Helper()
	return f.runJSONIn(dir, id.env(), args...)
}

func (f *Fixture) runJSONIn(dir string, extra []string, args ...string) any {
	f.t.Helper()
	r := f.exec(dir, extra, append([]string{"--json"}, args...)...)
	if r.ExitCode != 0 {
		f.t.Fatalf("ppwk --json %s 실패:\n%s", strings.Join(args, " "), r)
	}
	var envelope struct {
		Data any  `json:"data"`
		OK   bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &envelope); err != nil {
		f.t.Fatalf("JSON 파싱 (%s): %v\n%s", strings.Join(args, " "), err, r.Stdout)
	}
	if !envelope.OK {
		f.t.Fatalf("ok=false: %s", r.Stdout)
	}
	return envelope.Data
}

// Git 은 검증용 raw git 이다. CLI 가 거짓말을 해도 여기서 잡는다 (§0.2).
func (f *Fixture) Git(args ...string) string {
	f.t.Helper()
	return f.gitIn(f.Root, args...)
}

func (f *Fixture) GitIn(dir string, args ...string) string {
	f.t.Helper()
	return f.gitIn(dir, args...)
}

func (f *Fixture) gitIn(dir string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = baseEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %s (%s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

// Refs 는 주어진 prefix 아래 ref 를 이름 → OID 로 돌려준다.
func (f *Fixture) Refs(prefix string) map[string]string {
	f.t.Helper()
	out := f.Git("for-each-ref", "--format=%(refname) %(objectname)", prefix)
	refs := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if name, oid, ok := strings.Cut(line, " "); ok {
			refs[name] = oid
		}
	}
	return refs
}

// HasRef 는 ref 존재 여부다.
func (f *Fixture) HasRef(name string) bool {
	f.t.Helper()
	_, ok := f.Refs(name)[name]
	return ok
}

// waitFor 는 조건이 참이 될 때까지 폴링한다.
//
// sleep 을 쓰면 느리거나 flake 하거나 둘 다다 (§1.2). 이 헬퍼 하나만 쓴다.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s: %s 안에 성립하지 않았습니다", what, timeout)
}

// Agent 는 독립 프로세스로 살아 있는 에이전트 핸들이다.
//
// 잠금이 프로세스 수명에 묶이는지를 보려면 진짜 프로세스가 필요하다. 함수
// 호출로 흉내 내면 SIGKILL 이 무엇을 남기는지가 검증되지 않는다.
type Agent struct {
	t        *testing.T
	Identity Identity
	Dir      string
	fixture  *Fixture
	cmd      *exec.Cmd
	stopped  bool
}

// Agent 는 세션 훅을 실행한 뒤 그 부모 프로세스를 살려 둔다.
//
// 훅의 부모가 도구 프로세스라는 전제를 그대로 재현한다 (§3.8 층 3). sh 가
// 훅을 부르고 나서 sleep 으로 exec 하므로 PID 가 그대로 유지되고, 그 PID 가
// hook_pid 로 기록된다. SIGKILL 하면 OS 가 프로세스를 지우고 생존 판정이
// 즉시 사망으로 바뀐다 — 별도 정리 경로가 없다는 것이 요점이다.
func (f *Fixture) Agent(name string, dir string) *Agent {
	f.t.Helper()
	return f.AgentAs(Identity{Agent: name, Session: name + "-session"}, dir)
}

// AgentAs 는 세션 ID 까지 지정한다. 같은 이름으로 재시작하는 경우에 쓴다.
func (f *Fixture) AgentAs(id Identity, dir string) *Agent {
	f.t.Helper()
	a := &Agent{
		t:        f.t,
		fixture:  f,
		Identity: id,
		Dir:      dir,
	}
	payload, _ := json.Marshal(map[string]string{
		"session_id": a.Identity.Session, "cwd": dir, "hook_event_name": "SessionStart",
	})
	script := `printf '%s' "$PPWK_E2E_PAYLOAD" | "$PPWK_E2E_BIN" internal session-event; exec sleep 3600`
	a.cmd = exec.Command("sh", "-c", script)
	a.cmd.Dir = dir
	a.cmd.Env = append(append(baseEnv(), a.Identity.env()...),
		"PPWK_E2E_BIN="+binary, "PPWK_E2E_PAYLOAD="+string(payload))
	a.cmd.Stdout, a.cmd.Stderr = io.Discard, io.Discard
	if err := a.cmd.Start(); err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(a.Stop)

	// 훅이 기록을 남길 때까지 기다린다. Start() 는 sh 가 떴다는 뜻일 뿐이다.
	waitFor(f.t, 10*time.Second, id.Agent+" 세션 등록", a.HoldsLock)
	return a
}

// Kill 은 SIGKILL 이다. 정리 없이 즉사하며 OS 가 잠금을 해제한다.
func (a *Agent) Kill() {
	a.t.Helper()
	if a.cmd == nil || a.cmd.Process == nil || a.stopped {
		return
	}
	pid := a.cmd.Process.Pid
	if err := a.cmd.Process.Signal(syscall.SIGKILL); err != nil {
		a.t.Fatal(err)
	}
	a.cmd.Wait()
	a.stopped = true
	// 좀비로 남아 있는 동안에는 kill(pid, 0) 이 성공한다. 회수가 즉시
	// 일어나는지 보려면 커널이 PID 를 완전히 놓을 때까지 기다려야 한다.
	waitFor(a.t, 10*time.Second, fmt.Sprintf("pid %d 소멸", pid), func() bool {
		return syscall.Kill(pid, 0) != nil
	})
}

// Stop 은 SIGTERM 이다. 정상 종료 경로다.
func (a *Agent) Stop() {
	if a.cmd == nil || a.cmd.Process == nil || a.stopped {
		return
	}
	a.cmd.Process.Signal(syscall.SIGTERM)
	a.cmd.Wait()
	a.stopped = true
}

// SessionEnd 는 도구가 정상 종료할 때 부르는 훅이다.
func (a *Agent) SessionEnd() {
	a.t.Helper()
	payload, _ := json.Marshal(map[string]string{
		"session_id": a.Identity.Session, "cwd": a.Dir, "hook_event_name": "SessionEnd",
	})
	cmd := exec.Command(binary, "internal", "session-event")
	cmd.Dir = a.Dir
	cmd.Env = append(baseEnv(), a.Identity.env()...)
	cmd.Stdin = strings.NewReader(string(payload))
	if out, err := cmd.CombinedOutput(); err != nil {
		a.t.Fatalf("SessionEnd: %v\n%s", err, out)
	}
}

// Run 은 이 에이전트의 신원으로 명령을 실행한다.
func (a *Agent) Run(args ...string) Result {
	a.t.Helper()
	return a.fixture.RunAs(a.Identity, a.Dir, args...)
}

func (a *Agent) RunJSON(args ...string) any {
	a.t.Helper()
	return a.fixture.RunJSONAs(a.Identity, a.Dir, args...)
}

// HoldsLock 은 이 에이전트가 잠금 기록의 주인인지를 파일에서 직접 확인한다.
//
// CLI 를 거치지 않는다 — CLI 가 거짓말을 해도 여기서 잡아야 한다.
func (a *Agent) HoldsLock() bool {
	a.t.Helper()
	for _, lease := range a.fixture.leases() {
		if lease.Agent == a.Identity.Agent && lease.Session == a.Identity.Session {
			return true
		}
	}
	return false
}

// Lease 는 잠금 파일 한 건이다. 검증에 쓰는 필드만 둔다.
type Lease struct {
	Agent        string `json:"agent"`
	Session      string `json:"session"`
	Worktree     string `json:"worktree"`
	LastActivity string `json:"last_activity"`
	HookPID      *int   `json:"hook_pid"`
}

// leases 는 잠금 디렉터리의 에이전트 기록을 직접 읽는다.
func (f *Fixture) leases() []Lease {
	f.t.Helper()
	dir := filepath.Join(f.commonDir(), "ppwk", "locks")
	paths, _ := filepath.Glob(filepath.Join(dir, "agent-*.lock"))
	var out []Lease
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var lease Lease
		if json.Unmarshal(raw, &lease) == nil && lease.Agent != "" {
			out = append(out, lease)
		}
	}
	return out
}

func (f *Fixture) commonDir() string {
	f.t.Helper()
	dir := strings.TrimSpace(f.Git("rev-parse", "--path-format=absolute", "--git-common-dir"))
	return dir
}

// lines 는 출력에서 빈 줄을 뺀 목록이다.
func lines(s string) []string {
	var out []string
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		if t := strings.TrimSpace(scanner.Text()); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// chmod 는 실행 권한을 준다.
func chmod(path string, mode os.FileMode) error { return os.Chmod(path, mode) }

// execWithPath 는 PATH 앞에 dir 을 끼워 실행한다.
//
// 가짜 git 을 앞세워 버전 검사를 실제로 확인한다.
func (f *Fixture) execWithPath(dir string, args ...string) Result {
	f.t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = f.Root
	env := append(baseEnv(), f.Env...)
	for i, kv := range env {
		if name, value, _ := strings.Cut(kv, "="); name == "PATH" {
			env[i] = "PATH=" + dir + string(os.PathListSeparator) + value
		}
	}
	cmd.Env = env
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !asExitError(err, &exit) {
			f.t.Fatalf("ppwk %s: %v", strings.Join(args, " "), err)
		}
		code = exit.ExitCode()
	}
	return Result{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: code}
}
