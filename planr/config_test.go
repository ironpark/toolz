package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigHooks(t *testing.T) {
	root := t.TempDir()
	contents := []byte("plans_dir: plans\nignore:\n  - generated/**\nhooks:\n  before:\n    - on: [add, done]\n      run: echo before\n  after:\n    - on: [add, done]\n      run: echo after\n")
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), contents, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	value, foundRoot, err := loadConfig(root)
	if err != nil {
		t.Fatalf("loadConfig() unexpected error: %v", err)
	}
	if foundRoot != root {
		t.Fatalf("loadConfig() root = %q, want %q", foundRoot, root)
	}
	if len(value.Ignore) != 1 || value.Ignore[0] != "generated/**" {
		t.Fatalf("ignore = %#v, want [generated/**]", value.Ignore)
	}
	if got, want := value.Hooks.commands("before", hookEventAdd), []string{"echo before"}; !equalStrings(got, want) {
		t.Fatalf("before add hooks = %#v, want %#v", got, want)
	}
	if got, want := value.Hooks.commands("after", hookEventDone), []string{"echo after"}; !equalStrings(got, want) {
		t.Fatalf("after done hooks = %#v, want %#v", got, want)
	}
	if got := value.Hooks.commands("before", hookEventStart); len(got) != 0 {
		t.Fatalf("before start hooks = %#v, want empty", got)
	}
}

// Plan documents are written in the configured language; English is the
// default so an unconfigured repository is readable to anyone.
func TestLoadConfigLanguage(t *testing.T) {
	for _, testCase := range []struct {
		name, contents, want string
		wantError            string
	}{
		{name: "defaults to english", contents: "plans_dir: plans\n", want: languageEnglish},
		{name: "explicit korean", contents: "language: ko\n", want: languageKorean},
		{name: "normalizes case and spacing", contents: "language: \" KO \"\n", want: languageKorean},
		{name: "rejects unknown", contents: "language: fr\n", wantError: "not supported"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte(testCase.contents), 0644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			value, _, err := loadConfig(root)
			if testCase.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
					t.Fatalf("loadConfig() error = %v, want %q", err, testCase.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadConfig() unexpected error: %v", err)
			}
			if value.Language != testCase.want {
				t.Fatalf("language = %q, want %q", value.Language, testCase.want)
			}
		})
	}

	// A repository with no config file at all also defaults to English.
	value, _, err := loadConfig(t.TempDir())
	if err != nil {
		t.Fatalf("loadConfig() unexpected error: %v", err)
	}
	if value.Language != languageEnglish {
		t.Fatalf("language without a config file = %q, want %q", value.Language, languageEnglish)
	}
}

// The language setting picks the headings planr writes, and a draft using any
// supported language's headings must still parse.
func TestPlanDocumentsFollowConfiguredLanguage(t *testing.T) {
	for _, language := range sortedLanguages() {
		t.Run(language, func(t *testing.T) {
			text := documentStringsFor(language)
			plansRoot := t.TempDir()
			planRoot := filepath.Join(plansRoot, "00-checkout-v2")
			if err := writePlan(planRoot, testDraft(), "00-checkout-v2", language); err != nil {
				t.Fatalf("writePlan() unexpected error: %v", err)
			}
			phase := readFileString(t, filepath.Join(planRoot, "phases", "00-api-contract.md"))
			for _, want := range []string{"## " + text.plannedWork, "## " + text.doneWhen, "> NEXT: " + text.noNext} {
				if !strings.Contains(phase, want) {
					t.Errorf("phase document missing %q:\n%s", want, phase)
				}
			}
			plan := readFileString(t, filepath.Join(planRoot, "PLAN.md"))
			for _, want := range []string{"# " + text.verification, "# " + text.ordering, "# " + text.nextTarget} {
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
	valid := hookConfig{
		Before: []hookRule{{On: []string{hookEventAdd, hookEventDone}, Run: "echo check"}},
	}
	if err := validateHooks(valid); err != nil {
		t.Fatalf("validateHooks(valid) unexpected error: %v", err)
	}
	invalid := hookConfig{After: []hookRule{{On: []string{"unknown"}, Run: "echo check"}}}
	if err := validateHooks(invalid); err == nil || !strings.Contains(err.Error(), "unknown event") {
		t.Fatalf("validateHooks(invalid) error = %v, want unknown event", err)
	}
}

func TestLoadConfigRejectsUnknownHookSetting(t *testing.T) {
	root := t.TempDir()
	contents := []byte("hook:\n  phase_done: echo legacy\n")
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), contents, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, _, err := loadConfig(root); err == nil {
		t.Fatal("loadConfig() accepted an unknown hook setting")
	}
}

func TestIsIgnoredPath(t *testing.T) {
	patterns := []string{"generated/**", "tmp", "*.generated.go"}
	for _, path := range []string{"generated/build/app.go", "tmp/cache.bin", "main.generated.go"} {
		if !isIgnoredPath(path, patterns) {
			t.Errorf("isIgnoredPath(%q) = false, want true", path)
		}
	}
	if isIgnoredPath("cmd/main.go", patterns) {
		t.Error("isIgnoredPath(cmd/main.go) = true, want false")
	}
}

func TestRunHookEnvironment(t *testing.T) {
	root := t.TempDir()
	command := `printf '%s:%s:%s:%s' "$PLANR_EVENT" "$PLANR_PLAN" "$PLANR_PHASE" "$PLANR_STATUS" > hook.out`
	if err := runHook(root, command, "after done hook #1", hookEventDone, "00-checkout-v2", 2, "done"); err != nil {
		t.Fatalf("runHook() unexpected error: %v", err)
	}
	output, err := os.ReadFile(filepath.Join(root, "hook.out"))
	if err != nil {
		t.Fatalf("read hook output: %v", err)
	}
	if got, want := string(output), "done:00-checkout-v2:2:done"; got != want {
		t.Fatalf("hook output = %q, want %q", got, want)
	}
}

func TestRunHookPlanEventHasEmptyPhase(t *testing.T) {
	root := t.TempDir()
	command := `printf '<%s>' "$PLANR_PHASE" > hook.out`
	if err := runHook(root, command, "after add hook #1", hookEventAdd, "00-checkout-v2", -1, "registered"); err != nil {
		t.Fatalf("runHook() unexpected error: %v", err)
	}
	output, err := os.ReadFile(filepath.Join(root, "hook.out"))
	if err != nil {
		t.Fatalf("read hook output: %v", err)
	}
	if got, want := string(output), "<>"; got != want {
		t.Fatalf("hook output = %q, want %q", got, want)
	}
}

func TestRunConfiguredHooksPreservesRuleOrder(t *testing.T) {
	root := t.TempDir()
	settings := config{Hooks: hookConfig{After: []hookRule{
		{On: []string{hookEventAdd, hookEventDone}, Run: `printf 'one' >> hook.out`},
		{On: []string{hookEventDone}, Run: `printf 'two' >> hook.out`},
	}}}
	if err := runConfiguredHooks(root, settings, "after", hookEventDone, "00-checkout-v2", -1, "done"); err != nil {
		t.Fatalf("runConfiguredHooks() unexpected error: %v", err)
	}
	output, err := os.ReadFile(filepath.Join(root, "hook.out"))
	if err != nil {
		t.Fatalf("read hook output: %v", err)
	}
	if got, want := string(output), "onetwo"; got != want {
		t.Fatalf("hook output = %q, want %q", got, want)
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
