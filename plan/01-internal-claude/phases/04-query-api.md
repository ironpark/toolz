---
depends_on:
- "01-internal-claude#3"
perf_phase: false
status: planned
---
> DONE-WHEN: Tests against the fake transport: string prompt yields expected typed
> NEXT: none

# One-shot Query API

## Planned Work

- `query.go`: public `Query(ctx context.Context, prompt string, opts *Options)
  iter.Seq2[Message, error]` and `QueryStream(ctx, in iter.Seq[UserInput],
  opts *Options)` (streaming-input variant), porting `query.py` +
  `_internal/client.py process_query`: build transport (or accept an
  injected `Transport` via option for tests), construct engine, initialize
  when control-protocol features are needed (hooks/can_use_tool/sdk MCP —
  port `_has_bidirectional_needs` gating of `--input-format`), stream
  prompt(s), yield parsed messages until result, and guarantee cleanup on
  early break/cancellation (defer-close inside the range-func).
- Set entrypoint env marker for one-shot mode analogous to
  `CLAUDE_CODE_ENTRYPOINT=sdk-py` vs `sdk-py-client`.

## Done When

- Tests against the fake transport: string prompt yields expected typed
  message sequence ending in ResultMessage; early `break` from the range
  loop closes the transport and leaks no goroutines; context cancellation
  aborts; streaming-input variant writes each input line and ends input;
  `go test ./internal/claude` passes.
