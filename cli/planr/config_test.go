package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/ironpark/toolz/cli/planr/internal/agentenv"
	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
)

func TestLoadConfigHooks(t *testing.T) {
	root := t.TempDir()
	contents := []byte("plans_dir: plans\nignore:\n  - generated/**\nhooks:\n  before:\n    - on: [add, done]\n      run: echo before\n  after:\n    - on: [add, done]\n      run: echo after\n")
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), contents, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	value, foundRoot, err := config.Load(root)
	if err != nil {
		t.Fatalf("config.Load() unexpected error: %v", err)
	}
	if foundRoot != root {
		t.Fatalf("config.Load() root = %q, want %q", foundRoot, root)
	}
	if len(value.Ignore) != 1 || value.Ignore[0] != "generated/**" {
		t.Fatalf("ignore = %#v, want [generated/**]", value.Ignore)
	}
	if got, want := value.Hooks.Commands("before", hooks.EventAdd), []string{"echo before"}; !equalStrings(got, want) {
		t.Fatalf("before add hooks = %#v, want %#v", got, want)
	}
	if got, want := value.Hooks.Commands("after", hooks.EventDone), []string{"echo after"}; !equalStrings(got, want) {
		t.Fatalf("after done hooks = %#v, want %#v", got, want)
	}
	if got := value.Hooks.Commands("before", hooks.EventStart); len(got) != 0 {
		t.Fatalf("before start hooks = %#v, want empty", got)
	}
}

func TestLoadConfigStopsAtGitRepositoryRoot(t *testing.T) {
	parent := t.TempDir()
	if err := os.WriteFile(filepath.Join(parent, ".planr.yaml"), []byte("language: ko\nplans_dir: parent-plans\n"), 0644); err != nil {
		t.Fatalf("write parent config: %v", err)
	}
	repoRoot := filepath.Join(parent, "repo")
	if err := os.MkdirAll(filepath.Join(repoRoot, "nested"), 0755); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if _, err := git.PlainInit(repoRoot, false); err != nil {
		t.Fatalf("git init: %v", err)
	}

	value, foundRoot, err := config.Load(filepath.Join(repoRoot, "nested"))
	if err != nil {
		t.Fatalf("config.Load() unexpected error: %v", err)
	}
	if foundRoot != repoRoot {
		t.Fatalf("config.Load() root = %q, want %q", foundRoot, repoRoot)
	}
	if value.Language != doc.DefaultLanguage || len(value.PlansDirs) != 1 || value.PlansDirs[0] != "plan" {
		t.Fatalf("config.Load() crossed git root: %#v", value)
	}
	if value.Path != "" {
		t.Fatalf("default config unexpectedly has a path: %q", value.Path)
	}
	// The boundary also applies when a command is given a path that does not
	// exist yet; this keeps a parent config from being selected while creating
	// a new nested directory.
	value, foundRoot, err = config.Load(filepath.Join(repoRoot, "not-yet-created", "nested"))
	if err != nil {
		t.Fatalf("config.Load() for a missing nested path unexpected error: %v", err)
	}
	if foundRoot != repoRoot || value.Path != "" {
		t.Fatalf("missing nested path crossed git root: root=%q config=%q", foundRoot, value.Path)
	}
}

func TestLoadConfigKeepsAppliedPathAndHookTimeout(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".planr.yaml")
	contents := []byte("language: ko\nplans_dirs: [plans-active, plans-archive]\nignore: [tmp]\nhooks:\n  timeout: 25ms\n")
	if err := os.WriteFile(path, contents, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	value, foundRoot, err := config.Load(root)
	if err != nil {
		t.Fatalf("config.Load() unexpected error: %v", err)
	}
	if foundRoot != root || value.Path != path {
		t.Fatalf("config.Load() path/root = %q/%q, want %q/%q", value.Path, foundRoot, path, root)
	}
	if value.Hooks.Timeout != 25*time.Millisecond {
		t.Fatalf("hooks.timeout = %s, want 25ms", value.Hooks.Timeout)
	}
	if got := value.Hooks.TimeoutDuration(); got != 25*time.Millisecond {
		t.Fatalf("timeoutDuration() = %s, want 25ms", got)
	}
}

// Plan documents are written in the configured language; English is the
// default so an unconfigured repository is readable to anyone.
func TestLoadConfigLanguage(t *testing.T) {
	for _, testCase := range []struct {
		name, contents, want string
		wantError            string
	}{
		{name: "defaults to english", contents: "plans_dir: plans\n", want: doc.English},
		{name: "explicit korean", contents: "language: ko\n", want: doc.Korean},
		{name: "normalizes case and spacing", contents: "language: \" KO \"\n", want: doc.Korean},
		{name: "rejects unknown", contents: "language: fr\n", wantError: "not supported"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte(testCase.contents), 0644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			value, _, err := config.Load(root)
			if testCase.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
					t.Fatalf("config.Load() error = %v, want %q", err, testCase.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("config.Load() unexpected error: %v", err)
			}
			if value.Language != testCase.want {
				t.Fatalf("language = %q, want %q", value.Language, testCase.want)
			}
		})
	}

	// A repository with no config file at all also defaults to English.
	value, _, err := config.Load(t.TempDir())
	if err != nil {
		t.Fatalf("config.Load() unexpected error: %v", err)
	}
	if value.Language != doc.English {
		t.Fatalf("language without a config file = %q, want %q", value.Language, doc.English)
	}
	if value.Hooks.Timeout != hooks.DefaultTimeout {
		t.Fatalf("hook timeout without a config file = %s, want %s", value.Hooks.Timeout, hooks.DefaultTimeout)
	}
}

// The language setting picks the headings planr writes, and a draft using any
// supported language's headings must still parse.
func TestPlanDocumentsFollowConfiguredLanguage(t *testing.T) {
	for _, language := range doc.SortedLanguages() {
		t.Run(language, func(t *testing.T) {
			text := doc.StringsFor(language)
			plansRoot := t.TempDir()
			planRoot := filepath.Join(plansRoot, "00-checkout-v2")
			if err := plan.Write(planRoot, testDraft(), "00-checkout-v2", language); err != nil {
				t.Fatalf("plan.Write() unexpected error: %v", err)
			}
			phase := readFileString(t, filepath.Join(planRoot, "phases", "00-api-contract.md"))
			for _, want := range []string{"## " + text.PlannedWork, "## " + text.DoneWhen, "> NEXT: " + text.NoNext} {
				if !strings.Contains(phase, want) {
					t.Errorf("phase document missing %q:\n%s", want, phase)
				}
			}
			plan := readFileString(t, filepath.Join(planRoot, "PLAN.md"))
			for _, want := range []string{"# " + text.Verification, "# " + text.Ordering, "# " + text.NextTarget} {
				if !strings.Contains(plan, want) {
					t.Errorf("PLAN.md missing %q:\n%s", want, plan)
				}
			}
		})
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

func TestValidateHooks(t *testing.T) {
	valid := hooks.Config{
		Before: []hooks.Rule{{On: []string{hooks.EventAdd, hooks.EventDone}, Run: "echo check"}},
	}
	if err := hooks.Validate(valid); err != nil {
		t.Fatalf("hooks.Validate(valid) unexpected error: %v", err)
	}
	invalid := hooks.Config{After: []hooks.Rule{{On: []string{"unknown"}, Run: "echo check"}}}
	if err := hooks.Validate(invalid); err == nil || !strings.Contains(err.Error(), "unknown event") {
		t.Fatalf("hooks.Validate(invalid) error = %v, want unknown event", err)
	}
}

func TestLoadConfigRejectsUnknownHookSetting(t *testing.T) {
	root := t.TempDir()
	contents := []byte("hook:\n  phase_done: echo legacy\n")
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), contents, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, _, err := config.Load(root); err == nil {
		t.Fatal("config.Load() accepted an unknown hook setting")
	}
}

func TestIsIgnoredPath(t *testing.T) {
	patterns := []string{"generated/**", "tmp", "*.generated.go"}
	for _, path := range []string{"generated/build/app.go", "tmp/cache.bin", "main.generated.go"} {
		if !plan.IsIgnoredPath(path, patterns) {
			t.Errorf("plan.IsIgnoredPath(%q) = false, want true", path)
		}
	}
	if plan.IsIgnoredPath("cmd/main.go", patterns) {
		t.Error("plan.IsIgnoredPath(cmd/main.go) = true, want false")
	}
}

// runHookCapture runs one hook command in a scratch repository and returns
// whatever it wrote to hook.out, so each hook environment test is only its
// command and its expectation.
func runHookCapture(t *testing.T, event, command string, phaseID int, status string) string {
	t.Helper()
	root := t.TempDir()
	if err := hooks.RunOne(root, command, "after "+event+" hook #1", event, "00-checkout-v2", phaseID, status, hooks.DefaultTimeout); err != nil {
		t.Fatalf("hooks.RunOne() unexpected error: %v", err)
	}
	output, err := os.ReadFile(filepath.Join(root, "hook.out"))
	if err != nil {
		t.Fatalf("read hook output: %v", err)
	}
	return string(output)
}

func TestRunHookEnvironment(t *testing.T) {
	command := `printf '%s:%s:%s:%s' "$PLANR_EVENT" "$PLANR_PLAN" "$PLANR_PHASE" "$PLANR_STATUS" > hook.out`
	if got, want := runHookCapture(t, hooks.EventDone, command, 2, "done"), "done:00-checkout-v2:2:done"; got != want {
		t.Fatalf("hook output = %q, want %q", got, want)
	}
}

func TestDescribeAgent(t *testing.T) {
	detected := agentenv.Detection{
		Agent:     agentenv.AgentClaudeCode,
		SessionID: "session-7",
		Signal:    "CLAUDE_CODE_CHILD_SESSION",
		Level:     agentenv.DetectionDirect,
	}
	want := "claude-code (signal=CLAUDE_CODE_CHILD_SESSION, level=direct, session=session-7)"
	if got := agentenv.Describe(detected); got != want {
		t.Errorf("agentenv.Describe() = %q, want %q", got, want)
	}
	if got := agentenv.Describe(agentenv.Detection{}); !strings.HasPrefix(got, "none") {
		t.Errorf("agentenv.Describe(zero) = %q, want a none result", got)
	}
}

func TestRunHookExportsAgentEnvironment(t *testing.T) {
	// CLAUDE_CODE_CHILD_SESSION is the first marker Detect checks, so it wins
	// over whatever agent environment the test itself is running under.
	t.Setenv("CLAUDE_CODE_CHILD_SESSION", "1")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "session-7")
	command := `printf '%s:%s:%s' "$PLANR_AGENT" "$PLANR_AGENT_SESSION" "$PLANR_AGENT_LEVEL" > hook.out`
	if got, want := runHookCapture(t, hooks.EventDone, command, 2, "done"), "claude-code:session-7:direct"; got != want {
		t.Fatalf("hook output = %q, want %q", got, want)
	}
}

func TestRunHookPlanEventHasEmptyPhase(t *testing.T) {
	command := `printf '<%s>' "$PLANR_PHASE" > hook.out`
	if got, want := runHookCapture(t, hooks.EventAdd, command, -1, "registered"), "<>"; got != want {
		t.Fatalf("hook output = %q, want %q", got, want)
	}
}

func TestRunConfiguredHooksPreservesRuleOrder(t *testing.T) {
	root := t.TempDir()
	settings := config.Config{Hooks: hooks.Config{After: []hooks.Rule{
		{On: []string{hooks.EventAdd, hooks.EventDone}, Run: `printf 'one' >> hook.out`},
		{On: []string{hooks.EventDone}, Run: `printf 'two' >> hook.out`},
	}}}
	if err := hooks.Run(root, settings.Hooks, settings.SkipHooks, "after", hooks.EventDone, "00-checkout-v2", -1, "done"); err != nil {
		t.Fatalf("hooks.Run() unexpected error: %v", err)
	}
	output, err := os.ReadFile(filepath.Join(root, "hook.out"))
	if err != nil {
		t.Fatalf("read hook output: %v", err)
	}
	if got, want := string(output), "onetwo"; got != want {
		t.Fatalf("hook output = %q, want %q", got, want)
	}
}

func TestRunConfiguredHooksCanBeSkippedForOneInvocation(t *testing.T) {
	root := t.TempDir()
	settings := config.Config{
		SkipHooks: true,
		Hooks:     hooks.Config{After: []hooks.Rule{{On: []string{hooks.EventDone}, Run: "touch hook.out"}}},
	}
	if err := hooks.Run(root, settings.Hooks, settings.SkipHooks, "after", hooks.EventDone, "00-checkout-v2", 0, "done"); err != nil {
		t.Fatalf("hooks.Run() unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "hook.out")); !os.IsNotExist(err) {
		t.Fatalf("skipHooks still ran the hook; stat error = %v", err)
	}
}

func TestNoHooksGlobalFlagSkipsBeforeAndAfterHooks(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("plans_dir: plan\nhooks:\n  before:\n    - on: [start]\n      run: \"false\"\n  after:\n    - on: [start]\n      run: \"false\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	planRoot := filepath.Join(root, "plan", "00-checkout-v2")
	if err := plan.Write(planRoot, testDraft(), "00-checkout-v2", doc.English); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	command := newRootCommand()
	var output bytes.Buffer
	command.Writer = &output
	if err := command.Run(context.Background(), []string{"planr", "phase", "start", "checkout-v2", "0", "--no-hooks"}); err != nil {
		t.Fatalf("phase start --no-hooks failed: %v", err)
	}
	if got := frontmatterValue(t, filepath.Join(planRoot, "phases", "00-api-contract.md"), "status"); got != "in-progress" {
		t.Fatalf("phase status = %q, want in-progress", got)
	}
}

func TestRunConfiguredHooksUsesConfiguredTimeout(t *testing.T) {
	root := t.TempDir()
	settings := config.Config{Hooks: hooks.Config{
		Timeout: 20 * time.Millisecond,
		After:   []hooks.Rule{{On: []string{hooks.EventDone}, Run: "sleep 1"}},
	}}
	started := time.Now()
	err := hooks.Run(root, settings.Hooks, settings.SkipHooks, "after", hooks.EventDone, "00-checkout-v2", -1, "done")
	if err == nil || !strings.Contains(err.Error(), "timed out after 20ms") {
		t.Fatalf("hooks.Run() error = %v, want configured timeout", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("configured timeout took too long: %s", elapsed)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
