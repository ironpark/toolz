package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"
)

// Templates differ in what they put under test, not in how a trial is run:
// every one of them ends in a workspace, a prompt and a verification script.
var Templates = []string{"basic", "mcp-server", "cli-skill", "multi-agent"}

func initAction(_ context.Context, cmd *cli.Command) error {
	template := cmd.String("template")
	if !slices.Contains(Templates, template) {
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

	all := cmd.Bool("all")
	files := map[string]string{target: configTemplate(template)}
	if all || cmd.Bool("with-scripts") {
		files[filepath.Join(directory, "init.sh")] = initScriptTemplate
		files[filepath.Join(directory, "verify.sh")] = verifyScriptTemplate
	}
	if all || cmd.Bool("with-agent-md") {
		files[filepath.Join(directory, "AGENTS.md")] = agentMarkdownTemplate
	}
	if all || cmd.Bool("with-prompt") {
		files[filepath.Join(directory, "PROMPT.md")] = promptTemplate
	}
	if all || cmd.Bool("with-fixture") {
		// A directory alone would not survive a copy into a trial workspace,
		// and the template's own verify command looks for this README, so the
		// fixture is written with the file that makes it a working example.
		files[filepath.Join(directory, "fixture", "README.md")] = fixtureReadmeTemplate
	}
	// mcp.json is only referenced by the mcp-server template, so --all asks for
	// it there and nowhere else; --with-mcp writes it on request regardless.
	if cmd.Bool("with-mcp") || (all && template == "mcp-server") {
		files[filepath.Join(directory, "mcp.json")] = mcpConfigTemplate
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
	// Deterministic order so the created list reads the same on every run.
	for _, path := range slices.Sorted(maps.Keys(files)) {
		// Some templates write into subdirectories (the fixture workspace), so
		// each file's own parent is created rather than only the project root.
		if parent := filepath.Dir(path); parent != "" && parent != "." {
			if err := os.MkdirAll(parent, 0o755); err != nil {
				return err
			}
		}
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
  # Source-relative globs omitted from the isolated copy. A slashless pattern
  # matches a basename at any depth; ** crosses directory boundaries.
  # exclude: [FIXTURE.*]
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
  # name labels a prompt so a later one can come after it; a dependent prompt
  # is skipped when the prompt it comes after was never sent.
  - name: fix-build
    text: The build is broken. Fix it before you stop.
    when: sh("go build ./...") != 0
  - text: Add a regression test for the build fix.
    after: [fix-build]

# Commands run after the agent session ends and before verification. A bare
# string uses workspace scope; outside runs from the isolated scratch sibling.
# A failed command fails the trial, but later hooks and verification still run.
# hooks:
#   after:
#     - ./finalize.sh
#     - run: ./publish-summary.sh
#       scope: outside

verify:
  # Shell commands run in order outside the workspace once the agent stops,
  # with $MOHAE_WORKSPACE pointing at it. Each exits zero to pass; what it
  # prints is up to it.
  commands:
    - ./verify.sh
    - test -f "$MOHAE_WORKSPACE/README.md"

# Preserve selected output after verification and before a passing workspace
# is deleted. Paths and globs are relative to the workspace; missing matches
# are reported but do not grade the trial.
# artifacts:
#   - .harness/*.log

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
# The exit status is the verdict: zero passes, anything else fails. Print
# whatever helps a human read the result — mohae records the output verbatim
# and imposes no format on it.

set -uo pipefail

workspace="${MOHAE_WORKSPACE:?MOHAE_WORKSPACE is not set}"

if [ ! -f "$workspace/README.md" ]; then
  echo "README.md is missing" >&2
  exit 1
fi
`

// The first turn of the conversation. It lives outside the fixture on purpose:
// a prompt copied into the workspace would be a task file the agent can re-read
// on disk, and the trial is meant to measure what it does with what it was told.
const promptTemplate = `Describe the task the agent is being measured on.

State the goal and the constraints, and leave the approach to the agent: a
prompt that spells out the steps measures whether the agent can follow a
recipe, not whether it can solve the problem.
`

// The workspace the trial is run against. It is copied into an isolated
// directory before every trial, so what is written here is the state every run
// starts from.
const fixtureReadmeTemplate = `# Fixture workspace

Replace this with the repository the agent is measured on. It is copied to an
isolated directory before every trial, so a run never modifies what is here and
two runs of the same configuration start from identical state.
`

// The MCP servers offered to the agent, in the format the agent CLIs already
// read, so a server configuration can be shared with them unchanged.
const mcpConfigTemplate = `{
  "mcpServers": {
    "server-under-test": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-everything"]
    }
  }
}
`

const agentMarkdownTemplate = `# Working instructions

Instructions installed as AGENTS.md in the workspace.

Keep this document free of anything specific to one task or repository: a
trial measures whether the agent finds and follows these instructions, so the
same file should drop into any workspace unchanged.
`
