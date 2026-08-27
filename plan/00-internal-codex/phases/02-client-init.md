---
depends_on:
- "00-internal-codex#0"
- "00-internal-codex#1"
perf_phase: false
status: planned
---
> DONE-WHEN: Tests prove: successful handshake yields a usable client; initialize error
> NEXT: none

# Client and initialization handshake

## Planned Work

- `client.go`: `Client` type owning a Transport; `Options` struct (codex
  binary path defaulting to `codex` on PATH, extra args, env, ClientInfo
  name/title/version, optional notification opt-outs, logger).
- `New(ctx, opts)` (or `Dial`): spawns the subprocess, sends `initialize`
  with `clientInfo` and capabilities, waits for the result (user agent,
  platform info retained on the client), then sends the `initialized`
  notification; any failure kills the process and returns a wrapped error.
- Reject API use before handshake completes; make repeated `Close` idempotent;
  surface unexpected process exit as an error available via a `Done()`
  channel or `Err()` method.
- Internal notification router: dispatch by method prefix to thread/turn
  subscription registry (registry itself filled in by later phases) with a
  catch-all `OnNotification` hook for callers.
- `client_test.go`: fake app-server (reusing phase 0 pipe harness) asserting
  handshake ordering (`initialize` request answered before `initialized`
  notification is sent), error propagation when initialize fails, and clean
  Close.

## Done When

- Tests prove: successful handshake yields a usable client; initialize error
  or process death yields a constructor error with no goroutine leak; calls
  after Close return `ErrClosed`.
