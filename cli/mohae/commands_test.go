package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// run invokes the CLI the way a user would, so the flag wiring is covered
// rather than only the functions behind it.
func run(t *testing.T, arguments ...string) error {
	t.Helper()
	return newRootCommand().Run(context.Background(), append([]string{"mohae"}, arguments...))
}

func chdir(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(previous) })
	// macOS hands out symlinked temp paths; the resolved one is what the
	// commands will report back.
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestEveryCommandIsReachable(t *testing.T) {
	names := map[string]bool{}
	for _, command := range newRootCommand().Commands {
		names[command.Name] = true
	}
	for _, name := range []string{"run", "compare", "web", "init", "verify", "report"} {
		if !names[name] {
			t.Errorf("command %q is missing", name)
		}
	}
}

func TestInitWritesATemplateAndRefusesToClobber(t *testing.T) {
	directory := chdir(t)
	if err := run(t, "init", "--with-scripts", "--with-agent-md"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{DefaultConfigName, "init.sh", "verify.sh", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
			t.Errorf("%s was not created: %v", name, err)
		}
	}
	// A script mohae cannot execute would only surface at the end of a trial
	// that already cost tokens.
	for _, name := range []string{"init.sh", "verify.sh"} {
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&0o100 == 0 {
			t.Errorf("%s is not executable", name)
		}
	}
	if err := run(t, "init"); err == nil {
		t.Error("expected init to refuse to overwrite an existing config")
	}
	if err := run(t, "init", "--force"); err != nil {
		t.Errorf("--force should overwrite: %v", err)
	}
}

func TestInitTemplateIsAValidConfiguration(t *testing.T) {
	// The template is the first thing anyone runs; if it does not validate,
	// `mohae init && mohae verify` fails on its own output.
	directory := chdir(t)
	for _, template := range Templates {
		t.Run(template, func(t *testing.T) {
			if err := run(t, "init", "--force", "--template", template); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadConfig(filepath.Join(directory, DefaultConfigName)); err != nil {
				t.Fatalf("template %s does not validate: %v", template, err)
			}
		})
	}
}

func TestInitRejectsAnUnknownTemplate(t *testing.T) {
	chdir(t)
	if err := run(t, "init", "--template", "nonesuch"); err == nil {
		t.Fatal("expected an error for an unknown template")
	}
}

func TestVerifyReportsMissingPaths(t *testing.T) {
	chdir(t)
	if err := run(t, "init"); err != nil {
		t.Fatal(err)
	}
	// The template points at a fixture, a prompt and scripts that init did not
	// write, so verification has to fail rather than pass by omission.
	if err := run(t, "verify"); err == nil {
		t.Fatal("expected verification to fail on missing paths")
	}
}

func TestVerifyPassesOnceEverythingExists(t *testing.T) {
	directory := chdir(t)
	if err := run(t, "init", "--with-scripts", "--with-agent-md"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "PROMPT.md"), []byte("do the thing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(t, "verify", "--check-scripts", "--check-agent-md"); err != nil {
		t.Fatalf("verify = %v, want success", err)
	}
}

func TestVerifyStrictFailsOnWarnings(t *testing.T) {
	directory := chdir(t)
	if err := run(t, "init"); err != nil {
		t.Fatal(err)
	}
	// Drop the fields whose absence is a warning rather than a failure, and
	// satisfy everything else.
	config, err := os.ReadFile(filepath.Join(directory, DefaultConfigName))
	if err != nil {
		t.Fatal(err)
	}
	trimmed := strings.NewReplacer(
		"  init_script: ./init.sh\n", "",
		"  agent_md: ./AGENTS.md\n", "",
		"  commands:\n", "",
		"    - ./verify.sh\n", "",
		"    - test -f \"$MOHAE_WORKSPACE/README.md\"\n", "",
	).Replace(string(config))
	trimmed = strings.Replace(trimmed, "verify:\n", "", 1)
	if err := os.WriteFile(filepath.Join(directory, DefaultConfigName), []byte(trimmed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "PROMPT.md"), []byte("do the thing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(t, "verify"); err != nil {
		t.Fatalf("warnings alone should not fail: %v", err)
	}
	if err := run(t, "verify", "--strict"); err == nil {
		t.Fatal("--strict should turn warnings into a failure")
	}
}

func TestRunOverridesReachEveryConfig(t *testing.T) {
	directory := chdir(t)
	path := filepath.Join(directory, DefaultConfigName)
	if err := os.WriteFile(path, []byte(minimalConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	configs := []*Config{config}
	command := newRunCommand()
	if err := command.Run(context.Background(), []string{"run", "--agent", "claude-code", "--prompt", "inline", "--timeout", "42"}); !errors.Is(err, errNotImplemented) {
		t.Fatalf("run = %v, want a not-implemented error", err)
	}
	if err := applyRunOverrides(command, configs); err != nil {
		t.Fatal(err)
	}
	if config.Agent.Type != "claude-code" {
		t.Errorf("agent = %q", config.Agent.Type)
	}
	if len(config.Prompts) != 1 || config.Prompts[0].Text != "inline" || config.Prompts[0].File != "" {
		// An override that appended instead of replacing would send the
		// configured prompt too, and measure a conversation nobody typed.
		t.Errorf("prompts = %+v", config.Prompts)
	}
	if config.Limits.TimeoutSeconds != 42 {
		t.Errorf("timeout = %d", config.Limits.TimeoutSeconds)
	}
}

func TestRunRejectsTwoPromptSources(t *testing.T) {
	directory := chdir(t)
	if err := os.WriteFile(filepath.Join(directory, DefaultConfigName), []byte(minimalConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	err := run(t, "run", "--prompt", "inline", "--prompt-file", "PROMPT.md")
	if err == nil || errors.Is(err, errNotImplemented) {
		t.Fatalf("run = %v, want a mutual-exclusion error", err)
	}
}

func TestLoadConfigsExpandsGlobsAndDeduplicates(t *testing.T) {
	directory := chdir(t)
	for _, name := range []string{"a.config.yaml", "b.config.yaml"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(minimalConfig), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	configs, err := loadConfigs([]string{"*.config.yaml", "a.config.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 2 {
		t.Fatalf("loaded %d configs, want 2", len(configs))
	}
}

func TestLoadConfigsReportsTheMissingPathTheCallerTyped(t *testing.T) {
	chdir(t)
	_, err := loadConfigs([]string{"nope.yaml"})
	if err == nil || !strings.Contains(err.Error(), "nope.yaml") {
		t.Fatalf("err = %v, want it to name nope.yaml", err)
	}
}

func TestCompareValidatesItsArguments(t *testing.T) {
	cases := [][]string{
		{"compare", "--a", "x.yaml", "--b", "y.yaml", "--target", "nonesuch"},
		{"compare", "--a", "x.yaml", "--b", "y.yaml", "--metric", "vibes"},
		{"compare", "--a", "x.yaml", "--b", "y.yaml", "--repeat", "0"},
		{"compare", "--a", "same.yaml", "--b", "same.yaml"},
	}
	for _, arguments := range cases {
		t.Run(strings.Join(arguments[1:], " "), func(t *testing.T) {
			err := run(t, arguments...)
			if err == nil || errors.Is(err, errNotImplemented) {
				t.Fatalf("err = %v, want a validation error", err)
			}
		})
	}
}

func TestUnimplementedCommandsFailLoudly(t *testing.T) {
	// A skeleton that exited 0 would report success for work it never did.
	chdir(t)
	if err := os.WriteFile(DefaultConfigName, []byte(minimalConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{
		{"run"},
		{"compare", "--a", "a.yaml", "--b", "b.yaml"},
		{"report"},
	} {
		t.Run(arguments[0], func(t *testing.T) {
			if err := run(t, arguments...); !errors.Is(err, errNotImplemented) {
				t.Fatalf("err = %v, want a not-implemented error", err)
			}
		})
	}
}
