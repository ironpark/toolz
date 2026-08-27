---
completed_at: "2026-08-27T20:13:49Z"
perf_phase: false
status: done
---
> DONE-WHEN: `go test ./internal/codex/ -run Transport` passes; a scripted fake peer over
> NEXT: none

# JSON-RPC transport over subprocess stdio

## Planned Work

- `transport.go`: spawn `codex app-server` via `os/exec` with piped
  stdin/stdout (stderr passed through or captured for diagnostics);
  configurable binary path, args, env, and an `io.ReadWriteCloser` injection
  point so tests can substitute an in-memory pipe instead of a real process.
- JSONL framing: writer serializing one compact JSON object per line under a
  mutex; reader goroutine using `bufio.Scanner` with a generous max line size.
- Message model: raw envelope struct distinguishing request / response /
  notification / server-request by presence of `id`, `method`, `result`,
  `error`.
- `Call(ctx, method, params, result)` with atomic id generation and a pending
  map of reply channels; `Notify(method, params)`; a notification sink callback;
  a server-request handler callback that must reply with result or error.
- Shutdown semantics: `Close()` terminates the process, fails all pending
  calls with a sentinel error, and stops the reader; reader EOF/process exit
  propagates as a fatal transport error visible to callers.
- `transport_test.go`: unit tests over in-memory pipes covering call/response
  correlation, out-of-order responses, notifications, server-request replies,
  context cancellation, and close-with-pending-calls.

## Done When

- `go test ./internal/codex/ -run Transport` passes; a scripted fake peer over
  pipes exercises all four message kinds; canceled contexts return
  `ctx.Err()`; `Close` never leaks the reader goroutine (verified with a
  test using goroutine-count or channel completion).
