# GOALS

## Problem and the end result from the user's point of view

`mohae run` parses configurations, applies profiles and overrides, and then
stops at `notImplemented`. Nothing actually runs a trial. `mohae init` writes a
configuration whose own referenced files (fixture directory, PROMPT.md, MCP
config) it never creates, so `mohae init && mohae verify` fails on the tool's
own output.

When this plan is done, `mohae init --all` produces a project that verifies
clean, and `mohae run` copies the workspace, prepares it, drives the configured
agent through the conversation, grades the result with the verify commands, and
writes reports in the requested formats.

## Measurable goals

- `mohae init --all` followed by `mohae verify --check-scripts --check-agent-md`
  succeeds with no failures.
- `mohae run` on a custom-cli configuration runs the trial end to end and exits
  non-zero when verification fails.
- `--concurrency`, `--fail-fast`, `--show-dialogue`, `--detailed-tokens`,
  `--output` and `--report-dir` all change observable behaviour.
- `go build ./... && go vet ./... && go test ./...` pass offline, with no real
  `claude` or `codex` binary present.

## Supported scope and non-goals

In scope: the `init` and `run` commands, the workspace preparation, the three
agent drivers (claude-code, codex, custom-cli), MCP server wiring through
github.com/modelcontextprotocol/go-sdk, trial results, and report rendering.

Out of scope: `compare`, `web` and `report`, which stay unimplemented; the
`verify --check-mcp` flag; any change to the configuration schema, which is
already implemented and tested.

## Reference source / commit / license

In-repo only: internal/claude (Go port of claude-agent-sdk-python) and
internal/codex (codex app-server client) are complete and used as drivers.

## Completion criteria for the whole plan

Every phase is `done`, the tree is committed, and the build, vet and test
commands above pass.
