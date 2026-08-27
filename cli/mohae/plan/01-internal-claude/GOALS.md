# GOALS

## Problem and the end result from the user's point of view

mohae needs to drive the Claude Code CLI programmatically. The end result is a
Go package `internal/claude` inside `cli/mohae` that is a faithful, idiomatic
port of the official `claude-agent-sdk-python`: callers can run one-shot
queries and bidirectional interactive sessions against the `claude` CLI,
receive typed messages, register hooks and permission callbacks, and expose
in-process (SDK) MCP tools — all with Go contexts, channels/iterators, and
interfaces rather than translated Python idioms.

## Measurable goals

- `go build ./...` and `go vet ./...` pass with the new package.
- `Query(ctx, prompt, opts)` returns an iterator of typed messages for a
  string prompt or a streaming input source.
- `Client` supports Connect/Query/ReceiveMessages/ReceiveResponse/Interrupt/
  SetPermissionMode/SetModel/Disconnect against a live CLI.
- Control protocol handles initialize, interrupt, can_use_tool permission
  requests, hook callbacks, and mcp_message routing to in-process SDK MCP
  servers.
- Unit tests with a fake transport cover message parsing, control protocol
  round-trips, hooks, permissions, and SDK MCP tool dispatch; `go test ./...`
  passes without a real CLI installed.

## Supported scope and non-goals

Supported: message/content-block types, `Options`, subprocess transport with
stream-json protocol and CLI discovery, control protocol, one-shot Query API,
bidirectional Client API, in-process SDK MCP servers with typed tool handlers,
error types, tests. Non-goals: the Python SDK's session store / session
listing / session mutation / transcript-mirror subsystem (`sessions.py`,
`session_store.py`, `session_mutations.py`, `session_resume.py`,
`transcript_mirror_batcher.py`), the task/asyncio compat shims, Windows
`.cmd`-shim special-casing beyond a best-effort lookup, and any CLI bundling.

## Reference source / commit / license

Reference: `cli/mohae/ref/claude/claude-agent-sdk-python` (official Anthropic
Python SDK, MIT-licensed, vendored in-repo). Port the architecture described
by its `src/claude_agent_sdk/` package: `types.py`, `_errors.py`, `query.py`,
`client.py`, `_internal/transport/subprocess_cli.py`, `_internal/query.py`,
`_internal/message_parser.py`, `_internal/client.py`, and the SDK MCP support
in `__init__.py` / `_internal/sdk_mcp_bridge.py`.

## Completion criteria for the whole plan

All phases done; `go build ./...`, `go vet ./...`, `go test ./...` pass from
`cli/mohae`; a short doc comment on the package explains the public API; the
public surface covers query, client, options, messages, hooks, permissions,
SDK MCP tools, and errors.
