package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/goccy/go-yaml"
)

// Prompt is one user message in the trial conversation. A configuration lists
// them in order: the first opens the conversation, and each later one is sent
// as another turn once the agent has stopped producing output for the previous
// one. Prompts are deliberately not placed in the workspace, so the agent works
// from the conversation rather than from a task file it can re-read on disk.
//
// When is an optional expr expression. An empty When always sends; otherwise
// the prompt is sent only if the expression evaluates true against the state
// left by the preceding turns, which is what lets one configuration describe a
// follow-up that only some runs need ("if it never built, tell it to build").
type Prompt struct {
	Text string `yaml:"text,omitempty"`
	File string `yaml:"file,omitempty"`
	When string `yaml:"when,omitempty"`
	// ID names this prompt so later ones can depend on it. Optional: a prompt
	// nothing refers to does not need a name.
	ID string `yaml:"id,omitempty"`
	// DependsOn lists the IDs of earlier prompts this one requires. The prompt
	// is skipped unless every named prompt was actually sent, so a follow-up
	// to a conditional turn silently disappears with it instead of arriving
	// without its context.
	DependsOn []string `yaml:"depends_on,omitempty"`
	// TimeoutSeconds bounds this turn alone: the clock starts when the prompt
	// is sent, and the turn is cancelled once it runs out. Zero means the turn
	// has no limit of its own and only the trial-wide timeout applies.
	TimeoutSeconds int `yaml:"timeout_seconds,omitempty"`

	// program is the compiled When, populated by Compile so a typo in a
	// condition fails at load time rather than mid-trial after tokens are
	// already spent.
	program *vm.Program `yaml:"-"`
}

// UnmarshalYAML accepts either the full mapping or a bare string, so the common
// single-prompt case reads as `prompts: ["do the thing"]` instead of forcing a
// `text:` key onto every entry.
func (p *Prompt) UnmarshalYAML(data []byte) error {
	var text string
	if err := yaml.Unmarshal(data, &text); err == nil {
		p.Text = text
		return nil
	}
	// A distinct type: unmarshalling into Prompt here would recurse.
	type promptFields Prompt
	fields := promptFields{}
	if err := yaml.UnmarshalWithOptions(data, &fields, yaml.Strict()); err != nil {
		return err
	}
	*p = Prompt(fields)
	return nil
}

// Validate checks one prompt's shape and compiles its condition.
func (p *Prompt) Validate(field string) error {
	switch {
	case p.Text == "" && p.File == "":
		return fmt.Errorf("%s: text or file is required", field)
	case p.Text != "" && p.File != "":
		// Silently preferring one would make a trial measure a prompt nobody
		// meant to send.
		return fmt.Errorf("%s: text and file are mutually exclusive", field)
	}
	if p.TimeoutSeconds < 0 {
		return fmt.Errorf("%s.timeout_seconds must not be negative", field)
	}
	if p.When == "" {
		p.program = nil
		return nil
	}
	program, err := expr.Compile(p.When, expr.Env(PromptEnv{}), expr.AsBool())
	if err != nil {
		return fmt.Errorf("%s.when: %w", field, err)
	}
	p.program = program
	return nil
}

// DependenciesMet reports whether every prompt this one depends on was sent.
// sent maps a prompt ID to whether that turn actually ran.
func (p Prompt) DependenciesMet(sent map[string]bool) bool {
	for _, id := range p.DependsOn {
		if !sent[id] {
			return false
		}
	}
	return true
}

// ShouldSend reports whether this prompt's condition holds. An unconditional
// prompt is always sent.
func (p *Prompt) ShouldSend(env PromptEnv) (bool, error) {
	if p.program == nil {
		return true, nil
	}
	result, err := expr.Run(p.program, env)
	if err != nil {
		return false, fmt.Errorf("evaluating %q: %w", p.When, err)
	}
	send, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("evaluating %q: expected a boolean, got %T", p.When, result)
	}
	return send, nil
}

// PromptEnv is everything a `when` expression can see. The scalars describe the
// conversation so far; the functions look at the workspace the agent has been
// working in, which is the only way to condition on what the agent actually
// did rather than on what it said it did.
//
// Compiling against this type means an expression naming something that does
// not exist is rejected at load time, not at the turn it would have run.
type PromptEnv struct {
	// Turn is the 1-based position of this prompt in the list.
	Turn int `expr:"turn"`
	// Previous is the agent's last response, empty before the first turn.
	Previous string `expr:"previous"`
	// Responses is every response so far, oldest first.
	Responses []string `expr:"responses"`
	// ElapsedSeconds is the wall time the trial has consumed.
	ElapsedSeconds float64 `expr:"elapsed_seconds"`
	// TimedOut reports whether the trial's time limit has already been hit.
	TimedOut bool `expr:"timed_out"`

	// Exists reports whether a workspace-relative path is present.
	Exists func(string) bool `expr:"exists"`
	// Read returns a workspace-relative file's contents, or "" if unreadable.
	// Failure is not an error so a condition can be written as
	// `read("out.txt") contains "ok"` without guarding for a missing file.
	Read func(string) string `expr:"read"`
	// Sh runs a shell command in the workspace and returns its exit status, so
	// `sh("go build ./...") == 0` can gate a follow-up prompt.
	Sh func(string) int `expr:"sh"`
}

// NewPromptEnv builds an environment whose helpers are rooted at workspace.
func NewPromptEnv(workspace string) PromptEnv {
	resolve := func(path string) string {
		if filepath.IsAbs(path) {
			return path
		}
		return filepath.Join(workspace, path)
	}
	return PromptEnv{
		Exists: func(path string) bool {
			_, err := os.Stat(resolve(path))
			return err == nil
		},
		Read: func(path string) string {
			data, err := os.ReadFile(resolve(path))
			if err != nil {
				return ""
			}
			return string(data)
		},
		Sh: func(command string) int {
			cmd := exec.Command("sh", "-c", command)
			cmd.Dir = workspace
			if err := cmd.Run(); err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					return exitErr.ExitCode()
				}
				return -1
			}
			return 0
		},
	}
}

// TurnContext derives the context one turn runs under. The countdown starts
// here — at the moment the prompt is sent — so the returned context cancels
// the turn automatically once the prompt's own timeout elapses. A prompt
// without a timeout only inherits whatever deadline the trial context carries.
// The CancelFunc must be called once the turn ends to release the timer.
func (p Prompt) TurnContext(parent context.Context) (context.Context, context.CancelFunc) {
	if p.TimeoutSeconds <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, time.Duration(p.TimeoutSeconds)*time.Second)
}

// Describe renders a prompt for a plan listing: its source and its condition.
func (p Prompt) Describe() string {
	source := "file " + p.File
	if p.File == "" {
		source = fmt.Sprintf("text %q", truncate(p.Text, 40))
	}
	if p.ID != "" {
		source = p.ID + ": " + source
	}
	if len(p.DependsOn) > 0 {
		source += " after " + strings.Join(p.DependsOn, ", ")
	}
	if p.When != "" {
		source += " when " + p.When
	}
	if p.TimeoutSeconds > 0 {
		source += fmt.Sprintf(" (timeout %ds)", p.TimeoutSeconds)
	}
	return source
}

func truncate(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}
