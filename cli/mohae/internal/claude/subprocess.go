package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// Version is the SDK version reported to the CLI in CLAUDE_AGENT_SDK_VERSION.
const Version = "0.1.0"

// entrypoint and entrypointClient are reported to the CLI in
// CLAUDE_CODE_ENTRYPOINT, distinguishing one-shot queries from interactive
// client sessions.
const (
	entrypoint       = "sdk-go"
	entrypointClient = "sdk-go-client"
)

// stderrTailLimit caps how much stderr is retained for error reports.
const stderrTailLimit = 8 * 1024

// subprocessTransport runs the Claude Code CLI as a child process and speaks
// stream-json over its stdin and stdout.
type subprocessTransport struct {
	opts *Options

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	ready   bool
	exitErr error
	closed  bool

	cliPath string

	waitOnce sync.Once
	waitErr  error

	stderrMu   sync.Mutex
	stderrTail []byte
	stderrDone chan struct{}

	cancel context.CancelFunc
}

// newSubprocessTransport builds a transport for the given options. The CLI is
// located and the command line built at Connect time.
func newSubprocessTransport(opts *Options) *subprocessTransport {
	if opts == nil {
		opts = &Options{}
	}
	return &subprocessTransport{opts: opts}
}

// Connect locates the CLI, builds its command line and starts it.
func (t *subprocessTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cmd != nil {
		return nil
	}
	if t.opts.User != "" {
		return NewConnectionError(
			"Options.User is not supported by the subprocess transport; " +
				"run the process as the desired user instead")
	}

	cliPath := t.opts.CLIPath
	if cliPath == "" {
		if t.opts.Command != nil {
			// Discovery would answer with this host's copy, which is not the
			// one about to run. The bare name is resolved wherever the builder
			// starts the process.
			cliPath = defaultCLIName
		} else {
			found, err := findCLI()
			if err != nil {
				return err
			}
			cliPath = found
		}
	}
	t.cliPath = cliPath

	args, err := buildCommandArgs(t.opts)
	if err != nil {
		return err
	}

	// Only meaningful for a local process: with a builder, Cwd is a path in
	// whatever namespace the builder runs the CLI in, and this host's
	// filesystem has nothing to say about it.
	if t.opts.Cwd != "" && t.opts.Command == nil {
		if info, err := os.Stat(t.opts.Cwd); err != nil || !info.IsDir() {
			return NewConnectionError("Working directory does not exist: " + t.opts.Cwd)
		}
	}

	// The process is bound to a cancellable child of ctx so that both an
	// explicit Close and a cancelled ctx terminate the CLI.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-runCtx.Done():
		}
	}()
	t.cancel = cancel

	var cmd *exec.Cmd
	if t.opts.Command != nil {
		cmd = t.opts.Command(runCtx, cliPath, args, t.opts.Cwd, buildEnv(t.opts))
	} else {
		cmd = exec.CommandContext(runCtx, cliPath, args...)
		cmd.Dir = t.opts.Cwd
		cmd.Env = buildEnv(t.opts)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return NewConnectionError("Failed to start Claude Code: " + err.Error())
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return NewConnectionError("Failed to start Claude Code: " + err.Error())
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return NewConnectionError("Failed to start Claude Code: " + err.Error())
	}

	if err := cmd.Start(); err != nil {
		cancel()
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return NewCLINotFoundError("Claude Code not found at", cliPath)
		}
		return NewConnectionError("Failed to start Claude Code: " + err.Error())
	}

	t.cmd = cmd
	t.stdin = stdin
	t.stdout = stdout
	t.ready = true
	t.stderrDone = make(chan struct{})
	go t.pumpStderr(stderr)
	return nil
}

// pumpStderr forwards the CLI's stderr to the callback line by line and keeps a
// tail for inclusion in process errors.
func (t *subprocessTransport) pumpStderr(r io.ReadCloser) {
	defer close(t.stderrDone)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), t.opts.bufferSize())
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n \t")
		if line == "" {
			continue
		}
		t.appendStderr(line)
		if t.opts.Stderr != nil {
			// Isolated per line: a panicking callback must not stop the
			// pump and silently drop every later line.
			func() {
				defer func() { _ = recover() }()
				t.opts.Stderr(line)
			}()
		}
	}
}

// wait reaps the child process exactly once and reports its status. It first
// drains the stderr pump, since exec closes the pipes when Wait returns.
func (t *subprocessTransport) wait() error {
	t.waitOnce.Do(func() {
		if t.stderrDone != nil {
			<-t.stderrDone
		}
		t.waitErr = t.cmd.Wait()
	})
	return t.waitErr
}

func (t *subprocessTransport) appendStderr(line string) {
	t.stderrMu.Lock()
	defer t.stderrMu.Unlock()
	t.stderrTail = append(t.stderrTail, line...)
	t.stderrTail = append(t.stderrTail, '\n')
	if len(t.stderrTail) > stderrTailLimit {
		t.stderrTail = t.stderrTail[len(t.stderrTail)-stderrTailLimit:]
	}
}

func (t *subprocessTransport) stderrSnapshot() string {
	t.stderrMu.Lock()
	defer t.stderrMu.Unlock()
	return strings.TrimSpace(string(t.stderrTail))
}

// Write sends one frame to the CLI's stdin.
func (t *subprocessTransport) Write(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ready || t.stdin == nil {
		return NewConnectionError("transport is not ready for writing")
	}
	if t.exitErr != nil {
		return NewConnectionError("Cannot write to process that exited with error: " + t.exitErr.Error())
	}
	if _, err := t.stdin.Write(data); err != nil {
		t.ready = false
		t.exitErr = err
		return NewConnectionError("Failed to write to process stdin: " + err.Error())
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		if _, err := t.stdin.Write([]byte{'\n'}); err != nil {
			t.ready = false
			t.exitErr = err
			return NewConnectionError("Failed to write to process stdin: " + err.Error())
		}
	}
	return nil
}

// EndInput closes the CLI's stdin.
func (t *subprocessTransport) EndInput() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stdin == nil {
		return nil
	}
	err := t.stdin.Close()
	t.stdin = nil
	t.ready = false
	if err != nil && !errors.Is(err, os.ErrClosed) {
		return err
	}
	return nil
}

// Ready reports whether the CLI is running and accepting writes.
func (t *subprocessTransport) Ready() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ready
}

// Close terminates the CLI and releases its resources.
func (t *subprocessTransport) Close() error {
	t.mu.Lock()
	if t.closed || t.cmd == nil {
		t.closed = true
		t.ready = false
		cancel := t.cancel
		t.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		return nil
	}
	t.closed = true
	t.ready = false
	stdin, cancel := t.stdin, t.cancel
	t.stdin = nil
	t.mu.Unlock()

	if stdin != nil {
		_ = stdin.Close()
	}
	// Cancelling kills the process; exec.Cmd's WaitDelay is not used because
	// the reader may still be draining stdout.
	if cancel != nil {
		cancel()
	}
	err := t.wait()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// A non-zero status after an explicit Close is expected.
		return nil
	}
	return err
}

// ReadMessages yields the CLI's newline-delimited JSON output.
func (t *subprocessTransport) ReadMessages() iter.Seq2[json.RawMessage, error] {
	return func(yield func(json.RawMessage, error) bool) {
		t.mu.Lock()
		stdout, cmd := t.stdout, t.cmd
		t.mu.Unlock()
		if stdout == nil || cmd == nil {
			yield(nil, NewConnectionError("not connected"))
			return
		}

		maxSize := t.opts.bufferSize()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, min(64*1024, maxSize)), maxSize)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "{") {
				// Some CLI builds write diagnostics such as
				// "[SandboxDebug] ..." to stdout; they carry no message.
				continue
			}
			if !json.Valid([]byte(line)) {
				yield(nil, NewJSONDecodeError(line, errors.New("invalid JSON")))
				return
			}
			if !yield(json.RawMessage(line), nil) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			if errors.Is(err, bufio.ErrTooLong) {
				msg := fmt.Sprintf(
					"JSON message exceeded maximum buffer size of %d bytes", maxSize)
				yield(nil, &JSONDecodeError{baseError{Msg: msg}, "", err})
				return
			}
			if !errors.Is(err, os.ErrClosed) {
				yield(nil, NewConnectionError("Failed to read from process stdout: "+err.Error()))
				return
			}
		}

		// Output is exhausted: reap the process and report a failure exit.
		waitErr := t.wait()
		t.mu.Lock()
		t.ready = false
		t.mu.Unlock()
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			code := exitErr.ExitCode()
			perr := NewProcessError(
				fmt.Sprintf("Command failed with exit code %d", code), &code, t.stderrSnapshot())
			t.mu.Lock()
			t.exitErr = perr
			t.mu.Unlock()
			yield(nil, perr)
		}
	}
}

// ---------------------------------------------------------------------------
// CLI discovery
// ---------------------------------------------------------------------------

// defaultCLIName is the executable's name, used both as the start of local
// discovery and as the whole answer when the process is started elsewhere.
const defaultCLIName = "claude"

// findCLI locates the claude executable on PATH or in the usual install
// locations.
func findCLI() (string, error) {
	name := defaultCLIName
	if runtime.GOOS == "windows" {
		name = "claude.exe"
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	for _, c := range cliCandidatesFn() {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, nil
		}
	}
	return "", NewCLINotFoundError(
		"Claude Code not found. Install with:\n"+
			"  npm install -g @anthropic-ai/claude-code\n"+
			"\nIf already installed locally, try:\n"+
			"  export PATH=\"$HOME/node_modules/.bin:$PATH\"\n"+
			"\nOr provide the path via Options.CLIPath", "")
}

// cliCandidatesFn is the candidate list used by findCLI; tests replace it.
var cliCandidatesFn = cliCandidates

// cliCandidates lists the usual install locations for the CLI, in the order
// they are probed.
func cliCandidates() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	if runtime.GOOS == "windows" {
		if home == "" {
			return nil
		}
		return []string{filepath.Join(home, ".local", "bin", "claude.exe")}
	}
	candidates := []string{"/usr/local/bin/claude"}
	if home == "" {
		return candidates
	}
	return append(candidates,
		filepath.Join(home, ".npm-global", "bin", "claude"),
		filepath.Join(home, ".local", "bin", "claude"),
		filepath.Join(home, "node_modules", ".bin", "claude"),
		filepath.Join(home, ".yarn", "bin", "claude"),
		filepath.Join(home, ".claude", "local", "claude"),
	)
}

// ---------------------------------------------------------------------------
// Command line construction
// ---------------------------------------------------------------------------

// buildCommandArgs renders the CLI arguments for opts. The SDK always drives
// the CLI in streaming-json mode on both directions, matching the reference
// SDKs, so large configuration (agents, hooks) can ride on the initialize
// control request instead of the command line.
func buildCommandArgs(opts *Options) ([]string, error) {
	args := []string{"--output-format", "stream-json", "--verbose"}

	switch sp := opts.SystemPrompt.(type) {
	case nil:
		args = append(args, "--system-prompt", "")
	case SystemPromptText:
		args = append(args, "--system-prompt", string(sp))
	case *SystemPromptFile:
		args = append(args, "--system-prompt-file", sp.Path)
	case *SystemPromptPreset:
		if sp.Append != "" {
			args = append(args, "--append-system-prompt", sp.Append)
		}
	}

	switch tools := opts.Tools.(type) {
	case nil:
	case ToolList:
		args = append(args, "--tools", strings.Join(tools, ","))
	case ToolsPreset:
		// The claude_code preset maps to the CLI's "default" tool set.
		args = append(args, "--tools", "default")
	}

	allowedTools, settingSources, err := applySkillsDefaults(opts)
	if err != nil {
		return nil, err
	}
	if len(allowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(allowedTools, ","))
	}
	if opts.MaxTurns != nil {
		args = append(args, "--max-turns", strconv.Itoa(*opts.MaxTurns))
	}
	if opts.MaxBudgetUSD != nil {
		args = append(args, "--max-budget-usd", strconv.FormatFloat(*opts.MaxBudgetUSD, 'g', -1, 64))
	}
	if len(opts.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools", strings.Join(opts.DisallowedTools, ","))
	}
	if opts.TaskBudget != nil {
		args = append(args, "--task-budget", strconv.Itoa(opts.TaskBudget.Total))
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.FallbackModel != "" {
		args = append(args, "--fallback-model", opts.FallbackModel)
	}
	if len(opts.Betas) > 0 {
		args = append(args, "--betas", strings.Join(opts.Betas, ","))
	}
	if opts.PermissionPromptToolName != "" {
		args = append(args, "--permission-prompt-tool", opts.PermissionPromptToolName)
	}
	if opts.PermissionMode != "" {
		args = append(args, "--permission-mode", opts.PermissionMode)
	}
	if opts.ContinueConversation {
		args = append(args, "--continue")
	}
	// The equals form binds a dash-leading value to its flag; in the
	// two-token form the CLI would parse it as a separate flag, which lets an
	// untrusted session name inject arbitrary options.
	if opts.Resume != "" {
		args = append(args, "--resume="+opts.Resume)
	}
	if opts.SessionID != "" {
		args = append(args, "--session-id="+opts.SessionID)
	}
	settings, err := buildSettingsValue(opts)
	if err != nil {
		return nil, err
	}
	if settings != "" {
		args = append(args, "--settings", settings)
	}
	for _, dir := range opts.AddDirs {
		args = append(args, "--add-dir", dir)
	}
	if opts.MCPConfigPath != "" {
		args = append(args, "--mcp-config", opts.MCPConfigPath)
	} else if len(opts.MCPServers) > 0 {
		// SDK servers are listed by name and type only; the instance stays
		// in this process and is reached over the control protocol.
		payload, err := json.Marshal(map[string]any{"mcpServers": opts.MCPServers})
		if err != nil {
			return nil, fmt.Errorf("claude: encoding mcp servers: %w", err)
		}
		args = append(args, "--mcp-config", string(payload))
	}
	if opts.IncludePartialMessages {
		args = append(args, "--include-partial-messages")
	}
	if opts.IncludeHookEvents {
		args = append(args, "--include-hook-events")
	}
	if opts.StrictMCPConfig {
		args = append(args, "--strict-mcp-config")
	}
	if opts.ForkSession {
		args = append(args, "--fork-session")
	}
	if opts.ResumeSessionAt != "" {
		args = append(args, "--resume-session-at="+opts.ResumeSessionAt)
	}
	if opts.ResumeDropsTurn != "" {
		args = append(args, "--resume-drops-turn="+opts.ResumeDropsTurn)
	}
	if settingSources != nil {
		args = append(args, "--setting-sources="+strings.Join(*settingSources, ","))
	}
	for _, p := range opts.Plugins {
		if p.Type != "" && p.Type != "local" {
			return nil, fmt.Errorf("claude: unsupported plugin type: %s", p.Type)
		}
		args = append(args, "--plugin-dir", p.Path)
	}
	for _, flag := range sortedKeys(opts.ExtraArgs) {
		value := opts.ExtraArgs[flag]
		switch {
		case value == nil:
			args = append(args, "--"+flag)
		case strings.HasPrefix(*value, "-"):
			args = append(args, "--"+flag+"="+*value)
		default:
			args = append(args, "--"+flag, *value)
		}
	}
	if opts.Thinking != nil {
		switch opts.Thinking.Type {
		case "adaptive":
			args = append(args, "--thinking", "adaptive")
		case "enabled":
			if opts.Thinking.BudgetTokens != nil {
				args = append(args, "--max-thinking-tokens", strconv.Itoa(*opts.Thinking.BudgetTokens))
			}
		case "disabled":
			args = append(args, "--thinking", "disabled")
		}
	} else if opts.MaxThinkingTokens != nil {
		args = append(args, "--max-thinking-tokens", strconv.Itoa(*opts.MaxThinkingTokens))
	}
	if opts.Effort != "" {
		args = append(args, "--effort", opts.Effort)
	}
	if opts.OutputFormat != nil && opts.OutputFormat["type"] == "json_schema" {
		if schema, ok := opts.OutputFormat["schema"]; ok && schema != nil {
			payload, err := json.Marshal(schema)
			if err != nil {
				return nil, fmt.Errorf("claude: encoding output schema: %w", err)
			}
			args = append(args, "--json-schema", string(payload))
		}
	}

	args = append(args, "--input-format", "stream-json")
	return args, nil
}

// buildSettingsValue renders --settings, merging Options.Sandbox into the
// settings object when both are present.
func buildSettingsValue(opts *Options) (string, error) {
	if opts.Settings == "" && opts.Sandbox == nil {
		return "", nil
	}
	if opts.Sandbox == nil {
		return opts.Settings, nil
	}
	settings := map[string]any{}
	if trimmed := strings.TrimSpace(opts.Settings); trimmed != "" {
		if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
			if err := json.Unmarshal([]byte(trimmed), &settings); err != nil {
				return "", fmt.Errorf("claude: parsing inline settings: %w", err)
			}
		} else if body, err := os.ReadFile(trimmed); err == nil {
			if err := json.Unmarshal(body, &settings); err != nil {
				return "", fmt.Errorf("claude: parsing settings file %s: %w", trimmed, err)
			}
		}
	}
	settings["sandbox"] = opts.Sandbox
	payload, err := json.Marshal(settings)
	if err != nil {
		return "", fmt.Errorf("claude: encoding settings: %w", err)
	}
	return string(payload), nil
}

// applySkillsDefaults computes the effective allowed tools and setting sources
// for Options.Skills. Enabling skills implies the Skill tool and filesystem
// settings discovery, so callers do not have to wire both up by hand.
func applySkillsDefaults(opts *Options) ([]string, *[]string, error) {
	allowed := append([]string(nil), opts.AllowedTools...)
	var sources *[]string
	if opts.SettingSources != nil {
		copied := append([]string(nil), *opts.SettingSources...)
		sources = &copied
	}
	if opts.Skills == nil {
		return allowed, sources, nil
	}
	switch skills := opts.Skills.(type) {
	case SkillsAll:
		if !slices.Contains(allowed, "Skill") {
			allowed = append(allowed, "Skill")
		}
	case SkillList:
		for _, name := range skills {
			if err := validateSkillName(name); err != nil {
				return nil, nil, err
			}
			rule := "Skill(" + name + ")"
			if !slices.Contains(allowed, rule) {
				allowed = append(allowed, rule)
			}
		}
	}
	if sources == nil {
		defaults := []string{SettingSourceUser, SettingSourceProject}
		sources = &defaults
	}
	return allowed, sources, nil
}

// validateSkillName rejects names that cannot ride safely in a Skill(name)
// permission rule, or that could never match a discovered skill.
func validateSkillName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("claude: skill names must be non-empty")
	}
	if name != strings.TrimSpace(name) {
		return fmt.Errorf("claude: invalid skill name %q: leading or trailing whitespace can never match", name)
	}
	if name == "*" {
		return errors.New(`claude: invalid skill name "*": use SkillsAll{} to enable every skill`)
	}
	if strings.HasSuffix(name, ":*") || strings.HasSuffix(name, " *") {
		return fmt.Errorf("claude: invalid skill name %q: wildcard suffixes are not allowed", name)
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("claude: invalid skill name %q: use the canonical name, not the slash-command form", name)
	}
	if strings.Contains(name, `\\`) || strings.HasSuffix(name, `\`) {
		return fmt.Errorf("claude: invalid skill name %q: backslash escapes are not allowed", name)
	}
	for _, r := range name {
		if r == '(' || r == ')' || r == ',' || r == '\ufeff' || unicode.IsControl(r) {
			return fmt.Errorf("claude: invalid skill name %q: parentheses, commas and control characters are not allowed", name)
		}
	}
	return nil
}

// buildEnv renders the child process environment.
func buildEnv(opts *Options) []string {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "CLAUDECODE" {
			// CLAUDECODE is dropped so an SDK-spawned CLI does not think
			// it is running inside a Claude Code parent.
			continue
		}
		env[k] = v
	}
	env["CLAUDE_CODE_ENTRYPOINT"] = entrypoint
	for k, v := range opts.Env {
		env[k] = v
	}
	env["CLAUDE_AGENT_SDK_VERSION"] = Version
	if opts.EnableFileCheckpointing {
		env["CLAUDE_CODE_ENABLE_SDK_FILE_CHECKPOINTING"] = "true"
	}
	if opts.Cwd != "" {
		env["PWD"] = opts.Cwd
	}
	out := make([]string, 0, len(env))
	for _, k := range sortedKeys(env) {
		out = append(out, k+"="+env[k])
	}
	return out
}

// sortedKeys returns m's keys in order, so rendered command lines and
// environments are deterministic.
func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
