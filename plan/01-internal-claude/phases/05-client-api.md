---
depends_on:
- "01-internal-claude#3"
perf_phase: false
status: planned
---
> DONE-WHEN: Fake-transport tests cover: connect/disconnect lifecycle, multi-turn
> NEXT: none

# Bidirectional Client API

## Planned Work

- `client.go`: public `Client` struct porting `client.py ClaudeSDKClient`:
  `NewClient(opts *Options)`, `Connect(ctx)` (with optional initial prompt),
  `Query(ctx, prompt string, sessionID string)` and `QueryStream` for
  follow-ups, `ReceiveMessages(ctx) iter.Seq2[Message, error]`,
  `ReceiveResponse(ctx)` (yields until and including next ResultMessage),
  `Interrupt(ctx)`, `SetPermissionMode(ctx, mode)`, `SetModel(ctx, model)`,
  `RewindFiles(ctx, userMessageID)`, `McpServerStatus(ctx)`,
  `ContextUsage(ctx)`, `ReconnectMcpServer` / `ToggleMcpServer` /
  `StopTask`, `ServerInfo()`, `Disconnect() error` — errors on use before
  Connect (`CLIConnectionError` equivalent).
- Idiomatic lifecycle instead of Python context manager: `Connect` returns
  the client usable until `Disconnect`/ctx cancellation; document `defer
  client.Disconnect()` pattern; safe concurrent use of control methods while
  receiving.

## Done When

- Fake-transport tests cover: connect/disconnect lifecycle, multi-turn
  query + ReceiveResponse termination at ResultMessage without consuming the
  following turn, interrupt and setter round-trips, use-before-connect
  errors, and concurrent control calls during receive; `go test
  ./internal/claude` passes.
