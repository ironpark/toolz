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
	Session string // 세션 ID. 비어 있으면 단발 실행
	// Source 는 값을 어디서 얻었는지다. doctor 가 감지 근거를 보여줄 때 쓴다.
	AgentSource   string
	SessionSource string
}

// Options 는 결정 순서의 상위 항목을 넣는다. 빈 값은 다음 단계로 넘어간다.
type Options struct {
	Flag     string // --agent
	Worktree string // 현재 worktree 경로
	// GitConfig 는 ppwk.agent 값을 돌려준다. nil 이면 건너뛴다.
	GitConfig func() string
}

// Resolve 는 §0.2 와 §0.2.1 의 결정 순서를 그대로 따른다.
//
//	agent:   --agent → PPWK_AGENT → 도구 감지 → git config ppwk.agent → <hostname>:<worktree>
//	session: PPWK_SESSION → 도구 세션 ID → (훅 등록 세션은 호출자가 채운다) → 없음
func Resolve(opts Options) Identity {
	detected := runby.Current()
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
			id.AgentSource = "tool detection (" + detected.Chain() + ")"
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
	case os.Getenv("PPWK_SESSION") != "":
		id.Session, id.SessionSource = os.Getenv("PPWK_SESSION"), "PPWK_SESSION"
	default:
		if session, ok := detected.SessionID(); ok {
			id.Session, id.SessionSource = session.Value, "tool session ("+session.Agent.String()+")"
		}
	}

	return id
}

// Interactive 는 진행 표시와 색상을 켤지 판단한다 (§0.4).
func Interactive() bool {
	return runby.Current().TTY.Interactive
}
