# SCOPE

- New package directory `cli/mohae/internal/codex` (module
  `github.com/ironpark/toolz/cli/mohae`), stdlib-only implementation
  (`os/exec`, `bufio`, `encoding/json`, `sync`, `context`).
- Files (indicative): `transport.go`, `types.go`, `client.go`, `thread.go`,
  `turn.go`, `approvals.go`, `auth.go`, `errors.go`, plus `*_test.go` and a
  fake-server test helper.
- No changes to existing commands, go.mod dependencies, or README.

# CONTEXT

## Current implementation and bottlenecks

cli/mohae is a flat `package main` CLI built on urfave/cli/v3 (`main.go`,
`run.go`, `verify.go`, ...); there is no `internal/` tree and no process
control or JSON-RPC code yet. Nothing currently talks to Codex.

## Target structure and invariants

- Layered design: `Transport` (framing + id correlation + dispatch) below
  `Client` (protocol semantics) below `Thread`/`Turn` helpers.
- One reader goroutine owns stdout; it dispatches responses to per-request
  reply channels, notifications to subscribers, and server-initiated requests
  to registered handlers. Writes are serialized by a mutex or writer goroutine.
- Wire format: one JSON object per line, no `jsonrpc` field; requests carry
  `id`, notifications do not. Server->client requests carry an `id` the client
  must answer.
- Every blocking call takes a `context.Context`; closing the client cancels
  in-flight calls and kills the subprocess. Event channels are closed on turn
  completion or client shutdown so `for range` loops terminate.
- Approval handlers are interfaces with a safe default (decline) when the
  caller registers none.
