package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigReadsAMultiTurnConversation(t *testing.T) {
	config, err := LoadConfig(writeConfig(t, `name: sample
agent:
  type: codex
workspace:
  source: ./fixture
prompts:
  - file: ./PROMPT.md
  - just make it compile
  - text: now write the tests
    when: turn > 2 and not timed_out
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Prompts) != 3 {
		t.Fatalf("prompts = %+v", config.Prompts)
	}
	// A bare string is the shorthand for a text prompt; without it every
	// single-line follow-up would need a `text:` key.
	if config.Prompts[1].Text != "just make it compile" || config.Prompts[1].File != "" {
		t.Errorf("shorthand prompt = %+v", config.Prompts[1])
	}
	if config.Prompts[2].When == "" {
		t.Errorf("condition = %+v", config.Prompts[2])
	}
}

func TestPromptConditionsGovernWhetherAPromptIsSent(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "out.txt"), []byte("built ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := NewPromptEnv(workspace)
	env.Turn = 2
	env.Previous = "I gave up on the tests."
	env.Responses = []string{"done", env.Previous}

	cases := map[string]bool{
		"":                                 true,
		"turn > 1":                         true,
		"turn > 5":                         false,
		`previous contains "gave up"`:      true,
		"len(responses) == 2":              true,
		`exists("out.txt")`:                true,
		`exists("missing.txt")`:            false,
		`read("out.txt") contains "built"`: true,
		// An unreadable file reads as empty rather than erroring, so a
		// condition does not have to guard for a file the agent never wrote.
		`read("missing.txt") == ""`:      true,
		`sh("exit 3") == 3`:              true,
		`sh("true") == 0`:                true,
		"timed_out": false,
	}
	for source, want := range cases {
		t.Run(source, func(t *testing.T) {
			prompt := Prompt{Text: "x", When: source}
			if err := prompt.Validate("prompts[0]"); err != nil {
				t.Fatal(err)
			}
			got, err := prompt.ShouldSend(env)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("ShouldSend = %v, want %v", got, want)
			}
		})
	}
}

func TestPromptConditionsAreCheckedAtLoadTime(t *testing.T) {
	// A condition naming something that does not exist has to be rejected
	// before a trial spends tokens getting to the turn that would run it.
	prompt := Prompt{Text: "x", When: `previuos contains "typo"`}
	err := prompt.Validate("prompts[1]")
	if err == nil || !strings.Contains(err.Error(), "prompts[1].when") {
		t.Fatalf("err = %v, want one naming the field", err)
	}
}

func TestRunReplacesTheWholeConversation(t *testing.T) {
	directory := chdir(t)
	path := filepath.Join(directory, DefaultConfigName)
	if err := os.WriteFile(path, []byte(minimalConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	command := newRunCommand()
	arguments := []string{
		"run",
		"--prompt", "first",
		"--prompt", "second",
		"--prompt-when", "",
		"--prompt-when", "turn == 2",
	}
	if err := command.Run(t.Context(), arguments); err == nil {
		t.Fatal("expected the unimplemented runner to report itself")
	}
	configs := []*Config{config}
	if err := applyRunOverrides(command, configs); err != nil {
		t.Fatal(err)
	}
	if len(config.Prompts) != 2 {
		t.Fatalf("prompts = %+v", config.Prompts)
	}
	if config.Prompts[0].Text != "first" || config.Prompts[0].When != "" {
		t.Errorf("first prompt = %+v", config.Prompts[0])
	}
	// Conditions attach by position, so the second --prompt-when lands on the
	// second prompt rather than on all of them.
	if config.Prompts[1].Text != "second" || config.Prompts[1].When != "turn == 2" {
		t.Errorf("second prompt = %+v", config.Prompts[1])
	}
}

func TestRunRejectsMoreConditionsThanPrompts(t *testing.T) {
	directory := chdir(t)
	if err := os.WriteFile(filepath.Join(directory, DefaultConfigName), []byte(minimalConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{
		{"run", "--prompt", "only one", "--prompt-when", "turn > 0", "--prompt-when", "turn > 1"},
		{"run", "--prompt-when", "turn > 0"},
	} {
		if err := run(t, arguments...); err == nil {
			t.Errorf("%v: expected an error", arguments)
		}
	}
}
