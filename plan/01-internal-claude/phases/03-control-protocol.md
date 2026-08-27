---
completed_at: "2026-08-27T20:27:32Z"
depends_on:
- "01-internal-claude#1"
- "01-internal-claude#2"
perf_phase: false
status: done
---
> DONE-WHEN: Tests with an in-memory fake Transport verify: request/response ID
> NEXT: none

# Control protocol engine

## Planned Work

- `control.go`: `queryEngine` struct porting `_internal/query.py` `Query`:
  owns a Transport, a reader goroutine demultiplexing raw messages into
  (a) incoming `control_request` (dispatch to handlers in per-request
  goroutines, reply with `control_response` success/error), (b)
  `control_response` matched to pending SDK-initiated requests by
  `request_id` (map + channel per pending call), (c) regular messages
  delivered on an internal buffered channel consumed via
  `Messages() iter.Seq2[Message, error]`.
- `Initialize(ctx)`: send `initialize` control request carrying hook
  registrations (event -> matchers with callback IDs) and capabilities;
  store the response (commands, output styles) for `ServerInfo`.
- Incoming request handlers: `can_use_tool` -> opts.CanUseTool with
  `ToolPermissionContext`, encode allow/deny (updatedInput,
  updatedPermissions, interrupt) exactly as the CLI expects; `hook_callback`
  -> look up callback ID, run, convert `HookJSONOutput` to wire format
  (`_convert_hook_output_for_cli` equivalent); `mcp_message` -> route to the
  SDK MCP server registry (stub until phase 6, returning method-not-found).
- Outgoing requests: `Interrupt`, `SetPermissionMode`, `SetModel`,
  `RewindFiles`, `McpServerStatus`/`GetContextUsage` (`mcp_status`,
  `context_usage`), `ReconnectMcpServer`, `ToggleMcpServer`, `StopTask` —
  each `(ctx)` with timeout honoring; `StreamInput(ctx, seq)` writing
  user-message JSON lines then `EndInput` in one-shot mode after result
  (port `wait_for_result_and_end_input` for string-prompt close semantics).
- Lifecycle: `Close()` cancels the reader, waits for in-flight control
  handler goroutines (WaitGroup), closes the transport, and terminates the
  message sequence; fatal read errors propagate to both pending control
  calls and the message sequence.

## Done When

- Tests with an in-memory fake Transport verify: request/response ID
  matching and concurrent outgoing requests; can_use_tool allow and deny
  round-trips with input updates; hook callback registration in the
  initialize payload and execution with output conversion; error
  `control_response` mapping to Go errors; close joins all goroutines
  (asserted via `goleak`-style check or WaitGroup completion with timeout);
  `go test ./internal/claude` passes.
