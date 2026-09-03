package session

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/runby"
)

// 이 패키지의 감지 테스트는 두 겹의 환경을 다룬다. runby 에게는 WithEnviron
// 으로 만들어 준 환경만 보이고, Resolve 자신의 os.Getenv 경로에는 t.Setenv 로
// 넣은 실제 환경이 보인다. 테스트 프로세스가 어떤 도구 아래에서 돌든(그리고
// 이 저장소는 실제로 그런 환경에서 개발된다) 결과가 같아야 하므로, 두 겹을
// 같은 표 하나로 맞춘다.
var ppwkEnvKeys = []string{"PPWK_AGENT", "PPWK_SESSION", "PPWK_ACTIVITY_TTL"}

// withEnv 는 주어진 변수만 존재하는 환경을 만든다. 나머지는 지운다.
func withEnv(t *testing.T, env map[string]string) []string {
	t.Helper()
	for _, key := range ppwkEnvKeys {
		if _, ok := env[key]; !ok {
			t.Setenv(key, "")
			os.Unsetenv(key)
		}
	}
	environ := make([]string, 0, len(env))
	for key, value := range env {
		t.Setenv(key, value)
		environ = append(environ, key+"="+value)
	}
	return environ
}

// resolve 는 통제된 환경에서 신원을 결정한다.
//
// runby.Current 는 프로세스당 한 번만 감지하고 캐시하므로 환경변수 표를
// 검증할 수 없다. 프로세스 트리와 TTY 도 끈다 — 이 테스트가 확인하려는 것은
// 환경변수 → 신원 매핑이지, 테스트가 어디서 실행됐는지가 아니다.
func resolve(t *testing.T, env map[string]string, opts Options) Identity {
	t.Helper()
	environ := withEnv(t, env)
	opts.Detect = func() runby.Result {
		return runby.Detect(runby.WithEnviron(environ), runby.WithoutProcessTree(), runby.WithoutTTY())
	}
	if opts.Worktree == "" {
		opts.Worktree = "/tmp/repo-a"
	}
	return Resolve(opts)
}

// T4.21/T4.22/T4.23 — 도구 감지가 agent-id 와 그 근거를 정한다.
func TestAgentDetection(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		env        map[string]string
		wantAgent  string
		wantSource string
	}{
		{"T4.21 claude-code", map[string]string{"CLAUDECODE": "1"}, "claude-code:repo-a", "CLAUDECODE"},
		{"T4.22 codex", map[string]string{"CODEX_THREAD_ID": "t-9"}, "codex:repo-a", "CODEX_THREAD_ID"},
		{"T4.23 폴백", nil, host + ":repo-a", "hostname fallback"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := resolve(t, tc.env, Options{})
			if id.Agent != tc.wantAgent || id.AgentSource != tc.wantSource {
				t.Fatalf("agent=%q source=%q, want %q / %q", id.Agent, id.AgentSource, tc.wantAgent, tc.wantSource)
			}
		})
	}
}

// T4.20 — 도구 세션 ID 를 세션으로 채택하고 그 근거를 밝힌다.
func TestSessionDetection(t *testing.T) {
	id := resolve(t, map[string]string{"CLAUDECODE": "1", "CLAUDE_CODE_SESSION_ID": "abc-123"}, Options{})
	if id.Session != "abc-123" || id.SessionSource != "CLAUDE_CODE_SESSION_ID" {
		t.Fatalf("session=%q source=%q", id.Session, id.SessionSource)
	}
}

// T4.24 — PPWK_AGENT 가 도구 감지보다 우선한다.
func TestEnvAgentBeatsDetection(t *testing.T) {
	id := resolve(t, map[string]string{"CLAUDECODE": "1", "PPWK_AGENT": "orchestrated-3"}, Options{})
	if id.Agent != "orchestrated-3" {
		t.Fatalf("agent=%q", id.Agent)
	}
	// T4.27 — doctor 가 보여줄 근거는 실제 출처여야 한다. 여기서 "--agent" 가
	// 나온다면 플래그가 환경변수를 삼킨 것이다 (cmd/root.go 의 EnvVars 회귀).
	if id.AgentSource != "PPWK_AGENT" {
		t.Fatalf("source=%q, want PPWK_AGENT", id.AgentSource)
	}
}

// T4.24 — --agent 는 환경변수보다도 우선하며 자기 이름으로 보고된다.
func TestFlagBeatsEnvAgent(t *testing.T) {
	id := resolve(t, map[string]string{"PPWK_AGENT": "from-env"}, Options{Flag: "from-flag"})
	if id.Agent != "from-flag" || id.AgentSource != "--agent" {
		t.Fatalf("agent=%q source=%q", id.Agent, id.AgentSource)
	}
}

// T4.25 — PPWK_SESSION 이 도구 세션 ID 보다 우선한다.
func TestEnvSessionBeatsDetection(t *testing.T) {
	id := resolve(t, map[string]string{
		"CLAUDECODE": "1", "CLAUDE_CODE_SESSION_ID": "detected", "PPWK_SESSION": "forced",
	}, Options{})
	if id.Session != "forced" || id.SessionSource != "PPWK_SESSION" {
		t.Fatalf("session=%q source=%q", id.Session, id.SessionSource)
	}
}

// T4.26 — 같은 세션 ID 로 실행된 여러 명령이 같은 세션으로 묶인다.
func TestSameSessionIDGroupsCommands(t *testing.T) {
	env := map[string]string{"CLAUDECODE": "1", "CLAUDE_CODE_SESSION_ID": "stable-1"}
	first, second := resolve(t, env, Options{}), resolve(t, env, Options{})
	if first.Session != second.Session {
		t.Fatalf("%q != %q", first.Session, second.Session)
	}
}

// git config 는 감지가 전부 실패했을 때만 본다 (§0.2).
func TestGitConfigFallbackOrder(t *testing.T) {
	id := resolve(t, nil, Options{GitConfig: func() string { return "named-in-config" }})
	if id.Agent != "named-in-config" || id.AgentSource != "git config ppwk.agent" {
		t.Fatalf("agent=%q source=%q", id.Agent, id.AgentSource)
	}

	id = resolve(t, map[string]string{"CLAUDECODE": "1"},
		Options{GitConfig: func() string { return "named-in-config" }})
	if id.Agent != "claude-code:repo-a" {
		t.Fatalf("git config 가 도구 감지를 이겼다: %q", id.Agent)
	}
}

// T4.16e — 기본 임계값은 8h 다 (D11).
//
// 짧게 줄이면 커밋 중인 에이전트의 이슈가 회수된다. 30분 등으로 바꾸는 변경은
// 반드시 이 테스트를 깨뜨려야 한다.
func TestDefaultActivityTTLIsEightHours(t *testing.T) {
	if DefaultActivityTTL != 8*time.Hour {
		t.Fatalf("DefaultActivityTTL=%s, want 8h", DefaultActivityTTL)
	}
	withEnv(t, nil)
	r := NewRegistry(t.TempDir(), t.TempDir(), Identity{})
	if r.TTL != 8*time.Hour {
		t.Fatalf("TTL=%s, want 8h", r.TTL)
	}
	// start 후 7시간 CLI 무호출 → 생존 유지.
	now := time.Now()
	r.Now = func() time.Time { return now }
	lease := leaseAt(now.Add(-7 * time.Hour))
	if !r.Alive(lease) {
		t.Fatal("7시간 무호출이 사망 판정되었다")
	}
}

// T4.16c — 프로세스 이름 조회·트리 탐색 코드가 존재하지 않는다 (§3.6, D10).
//
// 프로세스 이름으로 도구를 찾는 접근은 조용히 틀리고 이식성 비용이 크다.
// pid 재사용 판별에 필요한 것은 시작 시각뿐이다.
func TestNoProcessNameLookup(t *testing.T) {
	// ps 필드 이름은 인자 리터럴로만 나타나므로 따옴표째 찾는다. 맨 낱말로
	// 찾으면 commonDir/exec.Command 같은 무관한 식별자에 걸린다.
	banned := []string{
		"/proc/", "sysctl", "pgrep", "pidof",
		`"comm"`, `"command"`, `"args"`, `"ppid"`, `"-ef"`, `"ax"`,
	}
	for _, name := range sourceFiles(t) {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		// 주석은 이 규칙을 설명하느라 낱말을 담을 수밖에 없다. 코드만 본다.
		for i, line := range strings.Split(string(body), "\n") {
			code := line
			if idx := strings.Index(code, "//"); idx >= 0 {
				code = code[:idx]
			}
			for _, needle := range banned {
				if strings.Contains(strings.ToLower(code), needle) {
					t.Errorf("%s:%d: 금지된 프로세스 조회 %q: %s", name, i+1, needle, strings.TrimSpace(line))
				}
			}
		}
	}
}

func sourceFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
			out = append(out, e.Name())
		}
	}
	if len(out) == 0 {
		t.Fatal("검사할 소스가 없다")
	}
	return out
}
