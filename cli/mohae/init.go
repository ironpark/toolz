package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"
)

// Templates differ in what they put under test, not in how a trial is run:
// every one of them ends in a workspace, a prompt and a verification script.
var Templates = []string{"basic", "mcp-server", "cli-skill", "multi-agent"}

func newInitCommand() *cli.Command {
	return &cli.Command{
		Name:      "init",
		Usage:     "write a configuration template, optionally with its scripts and AGENTS.md",
		ArgsUsage: "[PATH]",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "template", Aliases: []string{"t"}, Value: "basic", Usage: "preset: basic, mcp-server, cli-skill, multi-agent"},
			&cli.BoolFlag{Name: "with-scripts", Usage: "also write init.sh and verify.sh"},
			&cli.BoolFlag{Name: "with-agent-md", Usage: "also write an AGENTS.md template"},
			&cli.BoolFlag{Name: "force", Aliases: []string{"f"}, Usage: "overwrite existing files"},
		},
		Action: initAction,
	}
}

func initAction(_ context.Context, cmd *cli.Command) error {
	template := cmd.String("template")
	if !contains(Templates, template) {
		return fmt.Errorf("unknown --template %q (one of: %s)", template, strings.Join(Templates, ", "))
	}
	target := cmd.Args().First()
	if target == "" {
		target = DefaultConfigName
	}
	// A directory argument means "set a project up in here"; a file argument
	// names the config itself. Deciding by what is on disk keeps `mohae init`
	// and `mohae init trials/` from needing different flags.
	directory := "."
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		directory = target
		target = filepath.Join(target, DefaultConfigName)
	} else if filepath.Ext(target) == "" {
		directory = target
		target = filepath.Join(target, DefaultConfigName)
	} else {
		directory = filepath.Dir(target)
	}

	files := map[string]string{target: configTemplate(template)}
	if cmd.Bool("with-scripts") {
		files[filepath.Join(directory, "init.sh")] = initScriptTemplate
		files[filepath.Join(directory, "verify.sh")] = verifyScriptTemplate
	}
	if cmd.Bool("with-agent-md") {
		files[filepath.Join(directory, "AGENTS.md")] = agentMarkdownTemplate
	}

	force := cmd.Bool("force")
	if !force {
		// Checked before anything is written: a partial `init` that stopped
		// halfway would leave a project the caller has to clean up by hand.
		for path := range files {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists (use --force to overwrite)", path)
			}
		}
	}
	if directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
	}
	for _, path := range sortedKeys(files) {
		mode := os.FileMode(0o644)
		if strings.HasSuffix(path, ".sh") {
			// A verification script mohae cannot execute would only be
			// discovered at the end of a trial that already cost tokens.
			mode = 0o755
		}
		if err := os.WriteFile(path, []byte(files[path]), mode); err != nil {
			return err
		}
		fmt.Printf("created %s\n", path)
	}
	return nil
}

func sortedKeys(files map[string]string) []string {
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	// Deterministic order so the created list reads the same on every run.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func configTemplate(template string) string {
	header := `# mohae trial configuration.
# Paths are resolved relative to this file. See ` + "`mohae verify`" + ` to check
# them before spending tokens on a run.
name: ` + template + `-trial
description: what this trial is meant to measure

agent:
  type: codex
  model: gpt-5.6-luna
  effort: medium

workspace:
  # Copied into an isolated directory before every trial, so a run never
  # modifies the source and repeated runs start from identical state.
  source: ./fixture
  init_script: ./init.sh
  # Installed as AGENTS.md in the workspace. Kept outside the fixture so one
  # document can be shared by every configuration.
  agent_md: ./AGENTS.md
  git: true

prompts:
  # The conversation, in order. Deliberately not placed in the workspace: the
  # agent works from what it was told, not from a task file it can re-read on
  # disk. More than one entry makes the trial multi-turn.
  # timeout_seconds bounds one turn alone: the clock starts when the prompt is
  # sent and the turn is cancelled once it runs out. Without it, only the
  # trial-wide limits.timeout_seconds applies.
  - file: ./PROMPT.md
    timeout_seconds: 120
  # A follow-up sent only when its condition holds. Conditions are expr
  # expressions over the conversation so far (turn, previous, responses,
  # elapsed_seconds, timed_out) and the workspace the agent
  # worked in (exists, read, sh).
  # id names a prompt so a later one can depend_on it; a dependent prompt is
  # skipped when the prompt it depends on was never sent.
  - id: fix-build
    text: The build is broken. Fix it before you stop.
    when: sh("go build ./...") != 0
  - text: Add a regression test for the build fix.
    depends_on: [fix-build]

verify:
  # Runs outside the workspace once the agent stops, and is never copied in, so
  # the agent cannot tailor its output to the checks.
  script: ./verify.sh

limits:
  timeout_seconds: 300

report:
  dir: .mohae/reports
  formats: [terminal, json]
`
	switch template {
	case "mcp-server":
		return header + `
mcp:
  # Each server may limit which agent types it is offered to; omitting
  # agents offers it to all of them.
  - name: server-under-test
    config: ./mcp.json
    agents: [claude-code, codex]
`
	case "cli-skill":
		return header + `
# Build and install the CLI under test from init.sh, which runs inside the
# isolated workspace before the agent starts. Building there rather than
# relying on the machine means the agent gets the current source:
#
#   go build -o "$PWD/bin/mytool" ./cmd/mytool
#   export PATH="$PWD/bin:$PATH"
`
	case "multi-agent":
		return header + `
# One profile per agent: a section a profile declares replaces the base
# section wholesale, everything else stays shared, and that sameness is what
# makes the comparison mean something. Run each variant with
#   mohae run --profile claude
profiles:
  claude:
    agent:
      type: claude-code
      model: claude-opus-5
`
	default:
		return header
	}
}

const initScriptTemplate = `#!/usr/bin/env bash
# Runs inside the isolated workspace before the agent starts.
# Use it to build dependencies, seed data or install the CLI under test.

set -euo pipefail
`

const verifyScriptTemplate = `#!/usr/bin/env bash
# Runs after the agent stops, from a scratch directory outside the workspace.
# $MOHAE_WORKSPACE points at the finished workspace.
#
# Print one line per check so mohae can tabulate them:
#   CHECK<TAB>name<TAB>PASS|FAIL<TAB>detail
# Exit non-zero if any check failed.

set -uo pipefail

workspace="${MOHAE_WORKSPACE:?MOHAE_WORKSPACE is not set}"
failures=0

check() {
  local name="$1" status="$2" detail="${3:-}"
  printf 'CHECK\t%s\t%s\t%s\n' "$name" "$status" "$detail"
  [ "$status" = PASS ] || failures=$((failures + 1))
}

if [ -f "$workspace/README.md" ]; then
  check readme PASS
else
  check readme FAIL "README.md is missing"
fi

exit $((failures > 0))
`

const agentMarkdownTemplate = `# Working instructions

Instructions installed as AGENTS.md in the workspace.

Keep this document free of anything specific to one task or repository: a
trial measures whether the agent finds and follows these instructions, so the
same file should drop into any workspace unchanged.
`
