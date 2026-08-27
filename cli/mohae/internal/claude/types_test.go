package claude

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestContentBlockJSONRoundTrip(t *testing.T) {
	t.Parallel()
	isErr := true
	cases := []struct {
		name  string
		block ContentBlock
		json  string
	}{
		{"text", &TextBlock{Text: "hello"}, `{"text":"hello"}`},
		{"thinking", &ThinkingBlock{Thinking: "hmm", Signature: "sig"}, `{"thinking":"hmm","signature":"sig"}`},
		{"tool_use", &ToolUseBlock{ID: "tu1", Name: "Read", Input: map[string]any{"file_path": "/tmp/x"}},
			`{"id":"tu1","name":"Read","input":{"file_path":"/tmp/x"}}`},
		{"tool_result", &ToolResultBlock{ToolUseID: "tu1", IsError: &isErr}, `{"tool_use_id":"tu1","is_error":true}`},
		{"server_tool_use", &ServerToolUseBlock{ID: "st1", Name: ServerToolWebSearch, Input: map[string]any{"query": "go"}},
			`{"id":"st1","name":"web_search","input":{"query":"go"}}`},
		{"server_tool_result", &ServerToolResultBlock{ToolUseID: "st1", Content: map[string]any{"type": "web_search_result"}},
			`{"tool_use_id":"st1","content":{"type":"web_search_result"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.block)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != tc.json {
				t.Fatalf("marshal = %s, want %s", got, tc.json)
			}
			if tc.block.BlockType() != tc.name {
				t.Fatalf("BlockType = %q, want %q", tc.block.BlockType(), tc.name)
			}
		})
	}
}

func TestModelUsageUnmarshal(t *testing.T) {
	t.Parallel()
	const raw = `{"inputTokens":10,"outputTokens":20,"cacheReadInputTokens":1,
	  "cacheCreationInputTokens":2,"webSearchRequests":0,"costUSD":0.5,
	  "contextWindow":200000,"maxOutputTokens":64000,"canonicalModel":"claude-opus-4-7"}`
	var mu ModelUsage
	if err := json.Unmarshal([]byte(raw), &mu); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if mu.InputTokens != 10 || mu.CostUSD != 0.5 || mu.CanonicalModel != "claude-opus-4-7" {
		t.Fatalf("unexpected usage: %+v", mu)
	}
}

func TestMessageOriginRoundTrip(t *testing.T) {
	t.Parallel()
	const raw = `{"kind":"peer","from":"agent://x","name":"X","fromSession":"s1","verifiedPeerPid":42}`
	var o MessageOrigin
	if err := json.Unmarshal([]byte(raw), &o); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if o.Kind != OriginPeer || o.From != "agent://x" || o.VerifiedPeerPID == nil || *o.VerifiedPeerPID != 42 {
		t.Fatalf("unexpected origin: %+v", o)
	}
	out, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) != raw {
		t.Fatalf("marshal = %s, want %s", out, raw)
	}
}

func TestPermissionUpdateWireFormat(t *testing.T) {
	t.Parallel()
	content := "npm test"
	cases := []struct {
		name string
		u    PermissionUpdate
		want string
	}{
		{
			"addRules",
			PermissionUpdate{
				Type:        PermissionUpdateAddRules,
				Rules:       []PermissionRuleValue{{ToolName: "Bash", RuleContent: &content}},
				Behavior:    BehaviorAllow,
				Destination: DestinationSession,
			},
			`{"behavior":"allow","destination":"session","rules":[{"ruleContent":"npm test","toolName":"Bash"}],"type":"addRules"}`,
		},
		{
			"setMode",
			PermissionUpdate{Type: PermissionUpdateSetMode, Mode: PermissionModeAcceptEdits},
			`{"mode":"acceptEdits","type":"setMode"}`,
		},
		{
			"addDirectories",
			PermissionUpdate{Type: PermissionUpdateAddDirectories, Directories: []string{"/tmp"}},
			`{"directories":["/tmp"],"type":"addDirectories"}`,
		},
		{
			// Fields that do not belong to the variant are dropped.
			"setModeIgnoresRules",
			PermissionUpdate{Type: PermissionUpdateSetMode, Mode: PermissionModePlan, Directories: []string{"/tmp"}},
			`{"mode":"plan","type":"setMode"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.u)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("marshal = %s, want %s", got, tc.want)
			}
			var back PermissionUpdate
			if err := json.Unmarshal(got, &back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if back.Type != tc.u.Type {
				t.Fatalf("round trip type = %q, want %q", back.Type, tc.u.Type)
			}
		})
	}
}

func TestHookOutputWireFormat(t *testing.T) {
	t.Parallel()
	yes := true
	timeout := 5000
	cases := []struct {
		name string
		out  HookOutput
		want string
	}{
		{"empty", HookOutput{}, `{}`},
		{"continue", HookOutput{Continue: &yes}, `{"continue":true}`},
		{"block", HookOutput{Decision: "block", Reason: "nope"}, `{"decision":"block","reason":"nope"}`},
		{
			"hookSpecific",
			HookOutput{HookSpecificOutput: map[string]any{"hookEventName": "PreToolUse", "permissionDecision": "allow"}},
			`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow"}}`,
		},
		{"async", HookOutput{Async: true, AsyncTimeout: &timeout}, `{"async":true,"asyncTimeout":5000}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.out)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("marshal = %s, want %s", got, tc.want)
			}
			var back HookOutput
			if err := json.Unmarshal(got, &back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if back.Decision != tc.out.Decision || back.Async != tc.out.Async {
				t.Fatalf("round trip = %+v, want %+v", back, tc.out)
			}
		})
	}
}

func TestMCPServerConfigJSON(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  MCPServerConfig
		want string
	}{
		{"stdio", &MCPStdioServerConfig{Command: "node", Args: []string{"srv.js"}}, `{"type":"stdio","command":"node","args":["srv.js"]}`},
		{"sse", &MCPSSEServerConfig{URL: "https://x/sse"}, `{"type":"sse","url":"https://x/sse"}`},
		{"http", &MCPHTTPServerConfig{URL: "https://x/mcp", Headers: map[string]string{"A": "b"}}, `{"type":"http","url":"https://x/mcp","headers":{"A":"b"}}`},
		{"sdk", &MCPSDKServerConfig{Name: "calc", Instance: struct{}{}}, `{"name":"calc","type":"sdk"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.cfg)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("marshal = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestOptionsBufferSize(t *testing.T) {
	t.Parallel()
	var nilOpts *Options
	if got := nilOpts.bufferSize(); got != DefaultMaxBufferSize {
		t.Fatalf("nil options buffer = %d, want %d", got, DefaultMaxBufferSize)
	}
	if got := (&Options{}).bufferSize(); got != DefaultMaxBufferSize {
		t.Fatalf("zero options buffer = %d, want %d", got, DefaultMaxBufferSize)
	}
	if got := (&Options{MaxBufferSize: 32}).bufferSize(); got != 32 {
		t.Fatalf("buffer = %d, want 32", got)
	}
}

func TestAgentDefinitionJSON(t *testing.T) {
	t.Parallel()
	got, err := json.Marshal(AgentDefinition{
		Description: "reviewer",
		Prompt:      "review code",
		Tools:       []string{"Read"},
		Model:       "sonnet",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"description":"reviewer","prompt":"review code","tools":["Read"],"model":"sonnet"}`
	if string(got) != want {
		t.Fatalf("marshal = %s, want %s", got, want)
	}
}

func TestErrorHierarchy(t *testing.T) {
	t.Parallel()

	notFound := NewCLINotFoundError("", "/usr/bin/claude")
	if notFound.Error() != "Claude Code not found: /usr/bin/claude" {
		t.Fatalf("message = %q", notFound.Error())
	}
	var connErr *ConnectionError
	if !errors.As(error(notFound), &connErr) {
		t.Fatal("CLINotFoundError should unwrap to *ConnectionError")
	}
	var sdkErr Error
	if !errors.As(error(notFound), &sdkErr) {
		t.Fatal("CLINotFoundError should satisfy claude.Error")
	}

	code := 2
	proc := NewProcessError("Command failed", &code, "boom")
	if proc.Error() != "Command failed (exit code: 2)\nError output: boom" {
		t.Fatalf("message = %q", proc.Error())
	}
	if proc.ExitCode == nil || *proc.ExitCode != 2 || proc.Stderr != "boom" {
		t.Fatalf("unexpected fields: %+v", proc)
	}
}

func TestResultError(t *testing.T) {
	t.Parallel()
	code := 1
	data := map[string]any{
		"subtype":          "error_max_turns",
		"errors":           []any{" limit reached ", "", 7},
		"result":           "API Error: overloaded",
		"api_error_status": float64(529),
		"terminal_reason":  "api_error",
		"session_id":       "sess-1",
	}
	err := NewResultError("Query failed", data, &code)
	if err.Subtype != "error_max_turns" {
		t.Fatalf("subtype = %q", err.Subtype)
	}
	if len(err.Errors) != 1 || err.Errors[0] != "limit reached" {
		t.Fatalf("errors = %#v", err.Errors)
	}
	if err.APIErrorStatus == nil || *err.APIErrorStatus != 529 {
		t.Fatalf("api status = %v", err.APIErrorStatus)
	}
	if err.TerminalReason != "api_error" || err.SessionID != "sess-1" {
		t.Fatalf("unexpected fields: %+v", err)
	}
	var proc *ProcessError
	if !errors.As(error(err), &proc) {
		t.Fatal("ResultError should unwrap to *ProcessError")
	}

	// A bare string errors field is tolerated.
	err = NewResultError("x", map[string]any{"errors": "oops"}, nil)
	if len(err.Errors) != 1 || err.Errors[0] != "oops" {
		t.Fatalf("errors = %#v", err.Errors)
	}
	// A missing payload is tolerated.
	if e := NewResultError("x", nil, nil); e.Subtype != "" || e.Errors != nil {
		t.Fatalf("unexpected: %+v", e)
	}
}

func TestJSONDecodeAndParseErrors(t *testing.T) {
	t.Parallel()
	inner := errors.New("unexpected token")
	line := "{" + string(make([]byte, 0)) + "not json"
	de := NewJSONDecodeError(line, inner)
	if !errors.Is(de, inner) {
		t.Fatal("JSONDecodeError should wrap its cause")
	}
	if de.Line != line {
		t.Fatalf("line = %q", de.Line)
	}

	pe := NewMessageParseError("missing type", json.RawMessage(`{"a":1}`))
	if pe.Error() != "missing type" || string(pe.Data) != `{"a":1}` {
		t.Fatalf("unexpected: %+v", pe)
	}
	var sdkErr Error
	if !errors.As(error(pe), &sdkErr) {
		t.Fatal("MessageParseError should satisfy claude.Error")
	}
}
