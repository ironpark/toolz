package agentenv

import "testing"

// stubEnv builds a getenv function over a fixed map so detection can be tested
// without touching the process environment.
func stubEnv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestDetectEnvVendorMarkers(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		env     map[string]string
		agent   Agent
		session string
		signal  string
		level   DetectionLevel
	}{
		{
			name:    "claude code child session",
			env:     map[string]string{"CLAUDE_CODE_CHILD_SESSION": "1", "CLAUDE_CODE_SESSION_ID": "abc"},
			agent:   AgentClaudeCode,
			session: "abc",
			signal:  "CLAUDE_CODE_CHILD_SESSION",
			level:   DetectionDirect,
		},
		{
			name:    "codex thread",
			env:     map[string]string{"CODEX_THREAD_ID": "thread-1"},
			agent:   AgentCodex,
			session: "thread-1",
			signal:  "CODEX_THREAD_ID",
			level:   DetectionDirect,
		},
		{
			name:   "generic agent value",
			env:    map[string]string{"AGENT": "goose@1.2.3", "AGENT_SESSION_ID": "s1"},
			agent:  AgentGoose,
			signal: "AGENT", level: DetectionAmbient,
			session: "s1",
		},
		{
			name:    "claudecode is ambient",
			env:     map[string]string{"CLAUDECODE": "1", "CLAUDE_CODE_SESSION_ID": "abc"},
			agent:   AgentClaudeCode,
			session: "abc",
			signal:  "CLAUDECODE",
			level:   DetectionAmbient,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := DetectEnv(stubEnv(testCase.env))
			if got.Agent != testCase.agent || got.SessionID != testCase.session ||
				got.Signal != testCase.signal || got.Level != testCase.level {
				t.Fatalf("DetectEnv() = %+v, want agent=%q session=%q signal=%q level=%v",
					got, testCase.agent, testCase.session, testCase.signal, testCase.level)
			}
		})
	}
}

func TestDetectEnvPrefersDirectMarkers(t *testing.T) {
	got := DetectEnv(stubEnv(map[string]string{
		"CLAUDECODE":      "1",
		"AGENT":           "cursor",
		"CODEX_THREAD_ID": "thread-1",
	}))
	if !got.Direct() || got.Agent != AgentCodex {
		t.Fatalf("DetectEnv() = %+v, want a direct codex detection", got)
	}
}

func TestDetectEnvWithoutAgent(t *testing.T) {
	got := DetectEnv(stubEnv(nil))
	if got.Detected() || got.Level.String() != "" {
		t.Fatalf("DetectEnv() = %+v, want no detection", got)
	}
}

func TestParseAgentKeepsUnknownName(t *testing.T) {
	if got := parseAgent(" Devin@2.0 "); got != Agent("devin") {
		t.Fatalf("parseAgent() = %q, want %q", got, "devin")
	}
}
