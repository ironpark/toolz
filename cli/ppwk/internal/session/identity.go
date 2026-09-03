// Package session 은 현재 실행 주체와 그 생존을 다룬다 (design §14.8).
//
// 지금은 신원 결정만 있다. 잠금 파일과 생존 판정은 단계 4 에서 들어온다.
package session

import (
	"os"
	"path/filepath"

	"github.com/ironpark/runby"
)

// Identity 는 한 번의 실행이 어떤 에이전트·세션에 속하는지다.
type Identity struct {
	Agent   string // 에이전트 ID
	Session string // 세션 ID. 감지되지 않으면 실행마다 새 nonce (§4.3)
	// Source 는 값을 어디서 얻었는지다. doctor 가 감지 근거를 보여줄 때 쓴다.
	AgentSource   string
	SessionSource string
}

// Options 는 결정 순서의 상위 항목을 넣는다. 빈 값은 다음 단계로 넘어간다.
type Options struct {
	Flag     string // --agent
	Worktree string // 현재 worktree 경로
	// Session 은 세션 ID 를 직접 지정한다. 도구 훅이 stdin 으로 받은
	// session_id 를 넣는 자리다 — 훅은 도구 프로세스가 아니라 그 자식으로
	// 실행되므로 환경변수 감지가 닿지 않는다 (§3.8 층 3).
	Session string
	// GitConfig 는 ppwk.agent 값을 돌려준다. nil 이면 건너뛴다.
	GitConfig func() string
	// Detect 는 도구 감지를 수행한다. nil 이면 runby.Current 다.
	//
	// runby.Current 는 프로세스당 한 번만 감지하고 결과를 캐시한다. 그래서
	// 환경변수를 바꿔 가며 감지를 검증하려면 이 자리가 필요하다 (T4.20~T4.24).
	Detect func() runby.Result
}

// Resolve 는 §0.2 와 §0.2.1 의 결정 순서를 그대로 따른다.
//
//	agent:   --agent → PPWK_AGENT → 도구 감지 → git config ppwk.agent → <hostname>:<worktree>
//	session: 훅 session_id → PPWK_SESSION → 도구 세션 ID → 생성한 nonce
func Resolve(opts Options) Identity {
	detect := opts.Detect
	if detect == nil {
		detect = runby.Current
	}
	detected := detect()
	id := Identity{}

	suffix := filepath.Base(opts.Worktree)

	switch {
	case opts.Flag != "":
		id.Agent, id.AgentSource = opts.Flag, "--agent"
	case os.Getenv("PPWK_AGENT") != "":
		id.Agent, id.AgentSource = os.Getenv("PPWK_AGENT"), "PPWK_AGENT"
	default:
		if primary, ok := detected.Primary(); ok {
			id.Agent = primary.Name.String() + ":" + suffix
			id.AgentSource = toolAgentSource(primary.Name.String(), detected.Chain())
			break
		}
		if opts.GitConfig != nil {
			if v := opts.GitConfig(); v != "" {
				id.Agent, id.AgentSource = v, "git config ppwk.agent"
				break
			}
		}
		host, err := os.Hostname()
		if err != nil {
			host = "unknown"
		}
		id.Agent, id.AgentSource = host+":"+suffix, "hostname fallback"
	}

	switch {
	case opts.Session != "":
		id.Session, id.SessionSource = opts.Session, "hook session_id"
	case os.Getenv("PPWK_SESSION") != "":
		id.Session, id.SessionSource = os.Getenv("PPWK_SESSION"), "PPWK_SESSION"
	default:
		if session, ok := detected.SessionID(); ok {
			id.Session, id.SessionSource = session.Value, toolSessionSource(session.Agent.String())
			break
		}
		// 감지되는 세션이 없어도 비워 두지 않는다. 세션 값은 commit content 에
		// 들어가 OID 를 갈라놓는 역할을 겸하므로 (§4.3), 비어 있으면 같은 초에
		// 같은 전이를 시도한 두 프로세스가 동일한 commit 을 만든다.
		id.Session, id.SessionSource = NewNonce(), "generated nonce"
	}

	return id
}

// 도구 감지가 성공했을 때, 그 근거가 된 환경 변수 이름을 provenance 로 쓴다.
// doctor 가 "왜 이 신원인가" 를 설명할 때 감지 체인보다 이쪽이 구체적이다.
var (
	agentEnvKeys = map[string][]string{
		"claude-code": {"CLAUDECODE"},
		"codex":       {"CODEX_THREAD_ID", "CODEX_SESSION_ID", "CODEX_SANDBOX"},
	}
	sessionEnvKeys = map[string][]string{
		"claude-code": {"CLAUDE_CODE_SESSION_ID"},
		"codex":       {"CODEX_THREAD_ID", "CODEX_SESSION_ID"},
	}
)

// envSource 는 설정된 첫 후보 환경 변수의 이름을 돌려준다. 없으면 fallback 이다.
func envSource(keys []string, fallback string) string {
	for _, key := range keys {
		if os.Getenv(key) != "" {
			return key
		}
	}
	return fallback
}

func toolAgentSource(agent, chain string) string {
	return envSource(agentEnvKeys[agent], "tool detection ("+chain+")")
}

func toolSessionSource(agent string) string {
	return envSource(sessionEnvKeys[agent], "tool session ("+agent+")")
}

// Interactive 는 진행 표시와 색상을 켤지 판단한다 (§0.4).
func Interactive() bool {
	return runby.Current().TTY.Interactive
}
