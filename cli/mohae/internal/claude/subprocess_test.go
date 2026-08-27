package claude

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBuildCommandArgsDefaults(t *testing.T) {
	t.Parallel()
	args, err := buildCommandArgs(&Options{})
	if err != nil {
		t.Fatalf("buildCommandArgs: %v", err)
	}
	want := []string{
		"--output-format", "stream-json", "--verbose",
		"--system-prompt", "",
		"--input-format", "stream-json",
	}
	if !slices.Equal(args, want) {
		t.Fatalf("args = %q, want %q", args, want)
	}
}

func TestBuildCommandArgsOptions(t *testing.T) {
	t.Parallel()
	maxTurns := 3
	budget := 1.5
	value := "v"
	dash := "-x"
	sources := []string{SettingSourceProject}
	cases := []struct {
		name string
		opts Options
		want []string // fragments that must appear in order
		not  []string
	}{
		{"systemPromptText", Options{SystemPrompt: SystemPromptText("be nice")},
			[]string{"--system-prompt", "be nice"}, nil},
		{"systemPromptPresetAppend", Options{SystemPrompt: &SystemPromptPreset{Append: "extra"}},
			[]string{"--append-system-prompt", "extra"}, []string{"--system-prompt"}},
		{"systemPromptFile", Options{SystemPrompt: &SystemPromptFile{Path: "/p.md"}},
			[]string{"--system-prompt-file", "/p.md"}, nil},
		{"toolsList", Options{Tools: ToolList{"Bash", "Read"}}, []string{"--tools", "Bash,Read"}, nil},
		{"toolsEmpty", Options{Tools: ToolList{}}, []string{"--tools", ""}, nil},
		{"toolsPreset", Options{Tools: ToolsPreset{}}, []string{"--tools", "default"}, nil},
		{"allowedTools", Options{AllowedTools: []string{"Bash", "Read"}}, []string{"--allowedTools", "Bash,Read"}, nil},
		{"disallowedTools", Options{DisallowedTools: []string{"Bash"}}, []string{"--disallowedTools", "Bash"}, nil},
		{"maxTurns", Options{MaxTurns: &maxTurns}, []string{"--max-turns", "3"}, nil},
		{"maxBudget", Options{MaxBudgetUSD: &budget}, []string{"--max-budget-usd", "1.5"}, nil},
		{"taskBudget", Options{TaskBudget: &TaskBudget{Total: 100}}, []string{"--task-budget", "100"}, nil},
		{"model", Options{Model: "opus", FallbackModel: "sonnet"},
			[]string{"--model", "opus", "--fallback-model", "sonnet"}, nil},
		{"betas", Options{Betas: []SDKBeta{SDKBetaContext1M}}, []string{"--betas", "context-1m-2025-08-07"}, nil},
		{"permission", Options{PermissionMode: PermissionModePlan, PermissionPromptToolName: "stdio"},
			[]string{"--permission-prompt-tool", "stdio", "--permission-mode", "plan"}, nil},
		{"continue", Options{ContinueConversation: true}, []string{"--continue"}, nil},
		// The equals form keeps a dash-leading value bound to its flag.
		{"resume", Options{Resume: "--evil"}, []string{"--resume=--evil"}, nil},
		{"sessionID", Options{SessionID: "sid"}, []string{"--session-id=sid"}, nil},
		{"resumeAt", Options{ResumeSessionAt: "u1", ResumeDropsTurn: "u2"},
			[]string{"--resume-session-at=u1", "--resume-drops-turn=u2"}, nil},
		{"settings", Options{Settings: "/s.json"}, []string{"--settings", "/s.json"}, nil},
		{"addDirs", Options{AddDirs: []string{"/a", "/b"}}, []string{"--add-dir", "/a", "--add-dir", "/b"}, nil},
		{"flags", Options{IncludePartialMessages: true, IncludeHookEvents: true, StrictMCPConfig: true, ForkSession: true},
			[]string{"--include-partial-messages", "--include-hook-events", "--strict-mcp-config", "--fork-session"}, nil},
		{"settingSources", Options{SettingSources: &sources}, []string{"--setting-sources=project"}, nil},
		{"plugins", Options{Plugins: []PluginConfig{{Type: "local", Path: "/p"}}}, []string{"--plugin-dir", "/p"}, nil},
		{"extraArgsValue", Options{ExtraArgs: map[string]*string{"foo": &value}}, []string{"--foo", "v"}, nil},
		{"extraArgsBool", Options{ExtraArgs: map[string]*string{"bar": nil}}, []string{"--bar"}, nil},
		{"extraArgsDash", Options{ExtraArgs: map[string]*string{"baz": &dash}}, []string{"--baz=-x"}, nil},
		{"thinkingAdaptive", Options{Thinking: &ThinkingConfig{Type: "adaptive"}}, []string{"--thinking", "adaptive"}, nil},
		{"thinkingEnabled", Options{Thinking: &ThinkingConfig{Type: "enabled", BudgetTokens: &maxTurns}},
			[]string{"--max-thinking-tokens", "3"}, nil},
		{"thinkingDisabled", Options{Thinking: &ThinkingConfig{Type: "disabled"}}, []string{"--thinking", "disabled"}, nil},
		{"maxThinkingTokens", Options{MaxThinkingTokens: &maxTurns}, []string{"--max-thinking-tokens", "3"}, nil},
		{"effort", Options{Effort: EffortHigh}, []string{"--effort", "high"}, nil},
		{"mcpConfigPath", Options{MCPConfigPath: "/mcp.json"}, []string{"--mcp-config", "/mcp.json"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := buildCommandArgs(&tc.opts)
			if err != nil {
				t.Fatalf("buildCommandArgs: %v", err)
			}
			joined := strings.Join(args, "\x00")
			if !strings.Contains(joined, strings.Join(tc.want, "\x00")) {
				t.Fatalf("args %q missing %q", args, tc.want)
			}
			for _, absent := range tc.not {
				if slices.Contains(args, absent) {
					t.Fatalf("args %q should not contain %q", args, absent)
				}
			}
			// Streaming mode is always negotiated on both directions.
			if !strings.HasSuffix(joined, "--input-format\x00stream-json") {
				t.Fatalf("args %q should end with the input format", args)
			}
		})
	}
}

func TestBuildCommandArgsMCPServers(t *testing.T) {
	t.Parallel()
	args, err := buildCommandArgs(&Options{MCPServers: map[string]MCPServerConfig{
		"calc": &MCPSDKServerConfig{Name: "calc", Instance: struct{}{}},
		"fs":   &MCPStdioServerConfig{Command: "node", Args: []string{"fs.js"}},
	}})
	if err != nil {
		t.Fatalf("buildCommandArgs: %v", err)
	}
	i := slices.Index(args, "--mcp-config")
	if i < 0 || i+1 >= len(args) {
		t.Fatalf("missing --mcp-config in %q", args)
	}
	var payload struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(args[i+1]), &payload); err != nil {
		t.Fatalf("mcp config is not JSON: %v", err)
	}
	sdk := payload.MCPServers["calc"]
	if sdk["type"] != "sdk" || sdk["name"] != "calc" {
		t.Fatalf("sdk server = %#v", sdk)
	}
	// The in-process instance must never reach the CLI.
	if _, ok := sdk["instance"]; ok {
		t.Fatalf("sdk server config leaked its instance: %#v", sdk)
	}
	if payload.MCPServers["fs"]["command"] != "node" {
		t.Fatalf("stdio server = %#v", payload.MCPServers["fs"])
	}
}

func TestBuildCommandArgsOutputFormatAndSandbox(t *testing.T) {
	t.Parallel()
	args, err := buildCommandArgs(&Options{
		OutputFormat: map[string]any{"type": "json_schema", "schema": map[string]any{"type": "object"}},
		Sandbox:      json.RawMessage(`{"enabled":true}`),
		Settings:     `{"model":"opus"}`,
	})
	if err != nil {
		t.Fatalf("buildCommandArgs: %v", err)
	}
	i := slices.Index(args, "--json-schema")
	if i < 0 || args[i+1] != `{"type":"object"}` {
		t.Fatalf("json schema flag missing in %q", args)
	}
	j := slices.Index(args, "--settings")
	if j < 0 {
		t.Fatalf("settings flag missing in %q", args)
	}
	var merged map[string]any
	if err := json.Unmarshal([]byte(args[j+1]), &merged); err != nil {
		t.Fatalf("settings is not JSON: %v", err)
	}
	if merged["model"] != "opus" {
		t.Fatalf("settings lost its original keys: %#v", merged)
	}
	sandbox, ok := merged["sandbox"].(map[string]any)
	if !ok || sandbox["enabled"] != true {
		t.Fatalf("sandbox not merged: %#v", merged)
	}
}

func TestApplySkillsDefaults(t *testing.T) {
	t.Parallel()
	// "all" enables the bare Skill tool and defaults the setting sources.
	allowed, sources, err := applySkillsDefaults(&Options{Skills: SkillsAll{}})
	if err != nil {
		t.Fatalf("applySkillsDefaults: %v", err)
	}
	if !slices.Contains(allowed, "Skill") {
		t.Fatalf("allowed = %q", allowed)
	}
	if sources == nil || !slices.Equal(*sources, []string{"user", "project"}) {
		t.Fatalf("sources = %v", sources)
	}

	// A list produces one rule per skill and keeps explicit sources.
	explicit := []string{SettingSourceLocal}
	allowed, sources, err = applySkillsDefaults(&Options{
		Skills:         SkillList{"pdf", "plugin:docx"},
		AllowedTools:   []string{"Read"},
		SettingSources: &explicit,
	})
	if err != nil {
		t.Fatalf("applySkillsDefaults: %v", err)
	}
	if !slices.Equal(allowed, []string{"Read", "Skill(pdf)", "Skill(plugin:docx)"}) {
		t.Fatalf("allowed = %q", allowed)
	}
	if !slices.Equal(*sources, explicit) {
		t.Fatalf("sources = %v", *sources)
	}

	// Unset skills change nothing.
	allowed, sources, err = applySkillsDefaults(&Options{AllowedTools: []string{"Read"}})
	if err != nil || sources != nil || !slices.Equal(allowed, []string{"Read"}) {
		t.Fatalf("unexpected: %q %v %v", allowed, sources, err)
	}
}

func TestValidateSkillName(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"", "  ", " pdf", "pdf ", "*", "plugin:*", "/pdf", `a\\b`, `a\`, "a,b", "a(b)", "a\x00b"} {
		if err := validateSkillName(bad); err == nil {
			t.Errorf("validateSkillName(%q) = nil, want error", bad)
		}
	}
	for _, good := range []string{"pdf", "plugin:docx", "my-skill_1"} {
		if err := validateSkillName(good); err != nil {
			t.Errorf("validateSkillName(%q) = %v", good, err)
		}
	}
}

func TestBuildEnv(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("MOHAE_TEST_KEEP", "yes")
	env := buildEnv(&Options{
		Env:                     map[string]string{"CLAUDE_CODE_ENTRYPOINT": "custom", "EXTRA": "1"},
		EnableFileCheckpointing: true,
		Cwd:                     "/tmp",
	})
	got := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		got[k] = v
	}
	if _, ok := got["CLAUDECODE"]; ok {
		t.Fatal("CLAUDECODE should be filtered out")
	}
	if got["MOHAE_TEST_KEEP"] != "yes" {
		t.Fatal("inherited env should be preserved")
	}
	// options.Env may override the entrypoint but never the SDK version.
	if got["CLAUDE_CODE_ENTRYPOINT"] != "custom" || got["EXTRA"] != "1" {
		t.Fatalf("env = %#v", got)
	}
	if got["CLAUDE_AGENT_SDK_VERSION"] != Version {
		t.Fatalf("version = %q", got["CLAUDE_AGENT_SDK_VERSION"])
	}
	if got["CLAUDE_CODE_ENABLE_SDK_FILE_CHECKPOINTING"] != "true" || got["PWD"] != "/tmp" {
		t.Fatalf("env = %#v", got)
	}
	// Default entrypoint when the caller does not override it.
	env = buildEnv(&Options{})
	if !slices.Contains(env, "CLAUDE_CODE_ENTRYPOINT="+entrypoint) {
		t.Fatal("default entrypoint missing")
	}
}

func TestFindCLI(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "claude")
	if runtime.GOOS == "windows" {
		stub += ".exe"
	}
	if err := os.WriteFile(stub, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// PATH wins.
	t.Setenv("PATH", dir)
	got, err := findCLI()
	if err != nil {
		t.Fatalf("findCLI: %v", err)
	}
	if got != stub {
		t.Fatalf("findCLI = %q, want %q", got, stub)
	}

	// Fallback to a known install location.
	t.Setenv("PATH", filepath.Join(dir, "empty"))
	restore := cliCandidatesFn
	t.Cleanup(func() { cliCandidatesFn = restore })
	cliCandidatesFn = func() []string { return []string{filepath.Join(dir, "missing"), stub} }
	got, err = findCLI()
	if err != nil || got != stub {
		t.Fatalf("findCLI = %q, %v", got, err)
	}

	// Nothing anywhere.
	cliCandidatesFn = func() []string { return []string{filepath.Join(dir, "missing")} }
	if _, err := findCLI(); err == nil {
		t.Fatal("expected an error")
	} else {
		var notFound *CLINotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("error = %T (%v)", err, err)
		}
		if !strings.Contains(notFound.Error(), "npm install -g @anthropic-ai/claude-code") {
			t.Fatalf("error should carry an install hint: %v", notFound)
		}
	}
}

// writeStub writes an executable /bin/sh script and returns its path.
func writeStub(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub CLI scripts need a POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func collect(t *testing.T, tr Transport) ([]string, error) {
	t.Helper()
	var lines []string
	for raw, err := range tr.ReadMessages() {
		if err != nil {
			return lines, err
		}
		lines = append(lines, string(raw))
	}
	return lines, nil
}

func TestSubprocessTransportReadsNDJSON(t *testing.T) {
	stub := writeStub(t, `
echo '{"type":"system","subtype":"init"}'
echo ''
echo '[SandboxDebug] not json'
printf '{"type":"result","subtype":"success"}'
`)
	tr := newSubprocessTransport(&Options{CLIPath: stub})
	if err := tr.Connect(t.Context()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer tr.Close()
	if !tr.Ready() {
		t.Fatal("transport should be ready after connect")
	}
	lines, err := collect(t, tr)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := []string{`{"type":"system","subtype":"init"}`, `{"type":"result","subtype":"success"}`}
	if !slices.Equal(lines, want) {
		t.Fatalf("lines = %q, want %q", lines, want)
	}
}

func TestSubprocessTransportWriteAndEndInput(t *testing.T) {
	// The stub echoes back whatever it is sent, so the round trip proves both
	// stdin framing and stdout reading.
	stub := writeStub(t, `while IFS= read -r line; do echo "$line"; done`)
	tr := newSubprocessTransport(&Options{CLIPath: stub})
	if err := tr.Connect(t.Context()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer tr.Close()

	// A frame without a trailing newline gets one appended.
	if err := tr.Write(t.Context(), []byte(`{"a":1}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := tr.Write(t.Context(), []byte("{\"b\":2}\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := tr.EndInput(); err != nil {
		t.Fatalf("end input: %v", err)
	}
	if tr.Ready() {
		t.Fatal("transport should not be ready after EndInput")
	}
	if err := tr.Write(t.Context(), []byte(`{"c":3}`)); err == nil {
		t.Fatal("write after EndInput should fail")
	}
	lines, err := collect(t, tr)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !slices.Equal(lines, []string{`{"a":1}`, `{"b":2}`}) {
		t.Fatalf("lines = %q", lines)
	}
}

func TestSubprocessTransportProcessError(t *testing.T) {
	stub := writeStub(t, `
echo '{"type":"system","subtype":"init"}'
echo 'boom happened' >&2
exit 3
`)
	var stderrLines []string
	tr := newSubprocessTransport(&Options{
		CLIPath: stub,
		Stderr:  func(line string) { stderrLines = append(stderrLines, line) },
	})
	if err := tr.Connect(t.Context()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer tr.Close()
	lines, err := collect(t, tr)
	if len(lines) != 1 {
		t.Fatalf("lines = %q", lines)
	}
	var perr *ProcessError
	if !errors.As(err, &perr) {
		t.Fatalf("error = %T (%v), want *ProcessError", err, err)
	}
	if perr.ExitCode == nil || *perr.ExitCode != 3 {
		t.Fatalf("exit code = %v", perr.ExitCode)
	}
	if !strings.Contains(perr.Stderr, "boom happened") {
		t.Fatalf("stderr = %q", perr.Stderr)
	}
	if !slices.Contains(stderrLines, "boom happened") {
		t.Fatalf("stderr callback saw %q", stderrLines)
	}
}

func TestSubprocessTransportOversizedLine(t *testing.T) {
	stub := writeStub(t, `
awk 'BEGIN { printf "{\"a\":\""; for (i = 0; i < 200; i++) printf "0123456789"; print "\"}" }'
`)
	tr := newSubprocessTransport(&Options{CLIPath: stub, MaxBufferSize: 64})
	if err := tr.Connect(t.Context()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer tr.Close()
	_, err := collect(t, tr)
	var de *JSONDecodeError
	if !errors.As(err, &de) {
		t.Fatalf("error = %T (%v), want *JSONDecodeError", err, err)
	}
	if !strings.Contains(err.Error(), "maximum buffer size") {
		t.Fatalf("error = %v", err)
	}
}

func TestSubprocessTransportInvalidJSON(t *testing.T) {
	stub := writeStub(t, `echo '{"type": broken}'`)
	tr := newSubprocessTransport(&Options{CLIPath: stub})
	if err := tr.Connect(t.Context()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer tr.Close()
	_, err := collect(t, tr)
	var de *JSONDecodeError
	if !errors.As(err, &de) {
		t.Fatalf("error = %T (%v), want *JSONDecodeError", err, err)
	}
	if de.Line != `{"type": broken}` {
		t.Fatalf("line = %q", de.Line)
	}
}

func TestSubprocessTransportCloseTerminates(t *testing.T) {
	stub := writeStub(t, `
echo '{"type":"system","subtype":"init"}'
sleep 30
`)
	tr := newSubprocessTransport(&Options{CLIPath: stub})
	if err := tr.Connect(t.Context()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range tr.ReadMessages() {
			// Drain until the process is killed.
		}
	}()
	if err := tr.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Close is idempotent.
	if err := tr.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("reader did not stop after Close")
	}
	if tr.Ready() {
		t.Fatal("transport should not be ready after Close")
	}
}

func TestSubprocessTransportContextCancelKills(t *testing.T) {
	stub := writeStub(t, `sleep 30`)
	ctx, cancel := context.WithCancel(t.Context())
	tr := newSubprocessTransport(&Options{CLIPath: stub})
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer tr.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range tr.ReadMessages() {
		}
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("cancelling the context did not stop the process")
	}
}

func TestSubprocessTransportConnectErrors(t *testing.T) {
	t.Parallel()
	// A missing working directory is reported before the process starts.
	tr := newSubprocessTransport(&Options{CLIPath: "/bin/echo", Cwd: filepath.Join(t.TempDir(), "nope")})
	err := tr.Connect(t.Context())
	var connErr *ConnectionError
	if !errors.As(err, &connErr) || !strings.Contains(err.Error(), "Working directory") {
		t.Fatalf("error = %v", err)
	}

	// Options.User is refused rather than silently ignored.
	tr = newSubprocessTransport(&Options{CLIPath: "/bin/echo", User: "nobody"})
	if err := tr.Connect(t.Context()); err == nil || !strings.Contains(err.Error(), "Options.User") {
		t.Fatalf("error = %v", err)
	}

	// A bad option is reported from Connect, not swallowed.
	tr = newSubprocessTransport(&Options{CLIPath: "/bin/echo", Skills: SkillList{"bad,name"}})
	if err := tr.Connect(t.Context()); err == nil {
		t.Fatal("expected a skill validation error")
	}
}

func TestSubprocessTransportReadBeforeConnect(t *testing.T) {
	t.Parallel()
	tr := newSubprocessTransport(&Options{})
	_, err := collect(t, tr)
	var connErr *ConnectionError
	if !errors.As(err, &connErr) {
		t.Fatalf("error = %T (%v)", err, err)
	}
}
