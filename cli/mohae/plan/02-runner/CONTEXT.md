# SCOPE

- cli/mohae/init.go — scaffolding for the files the template references.
- cli/mohae/run.go — the run action, concurrency, fail-fast, exit status.
- cli/mohae/workspace.go — isolated workspace copy, init script, AGENTS.md,
  skills, optional git.
- cli/mohae/internal/driver/ — the agent drivers behind one interface.
- cli/mohae/mcpwiring.go — MCP configuration files parsed once and handed to
  each driver in the form it wants.
- cli/mohae/trial.go — the turn loop and the verification stage.
- cli/mohae/result.go, render.go — trial results and their renderings.
- cli/mohae/README.md — Korean documentation of the behaviour that changed.

# CONTEXT

## Current implementation and bottlenecks

config.go, prompt.go and profile.go already model everything a trial needs:
`ShouldSend`, `DependenciesMet` and `TurnContext` exist and are tested, as does
`ApplyProfile`. What is missing is everything downstream of the configuration:
nothing copies a workspace, nothing talks to an agent, nothing grades a result
and nothing renders one.

commands_test.go asserts that `run` returns `errNotImplemented`; those
assertions move to the real behaviour as the runner lands.

## Target structure and invariants

- A trial never touches `workspace.source`: it is copied to a fresh directory
  per trial, and everything after that happens in the copy.
- Verification runs outside the workspace with `$MOHAE_WORKSPACE` set, so
  grading cannot leave files that look like the agent's work.
- A driver is one interface: start a session in a workspace, send a turn, get
  back text and usage. The runner has no knowledge of which agent it drives.
- Tests never require a real agent binary or the network: the custom-cli driver
  with shell stubs stands in for a real agent.
