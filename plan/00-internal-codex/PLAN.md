---
completed_at: "2026-08-27T20:42:19Z"
description: "Go package internal/codex: client controlling Codex via the app-server JSON-RPC protocol over a subprocess (transport, types, threads, turns, events, approvals)"
plan_status: done
registered_at: "2026-08-27T20:07:33Z"
---
> NEXT: Build the JSON-RPC stdio transport with subprocess management, framing, id correlation, and dispatch, tested over in-memory pipes. ([Phase 0](phases/00-jsonrpc-transport.md))

# Phases

- [x] [Phase 00: JSON-RPC transport over subprocess stdio](phases/00-jsonrpc-transport.md)
- [x] [Phase 01: Protocol types](phases/01-protocol-types.md)
- [x] [Phase 02: Client and initialization handshake](phases/02-client-init.md)
- [x] [Phase 03: Thread management](phases/03-thread-management.md)
- [x] [Phase 04: Turn execution and event streaming](phases/04-turn-streaming.md)
- [x] [Phase 05: Approvals and server-initiated requests](phases/05-approvals.md)
- [x] [Phase 06: Auth surface](phases/06-auth-surface.md)
- [x] [Phase 07: Fake-server hardening, docs, and package test sweep](phases/07-hardening-and-docs.md)

# Shared Verification

- Per phase: targeted `go test ./internal/codex/ -run <PhasePattern>` plus
  `go build ./...` and `go vet ./...` before `planr phase done`.
- Whole plan: `go test -race ./internal/codex/...` green; the Example usage
  code compiles; no real `codex` binary is required by any test (fake server
  over in-memory pipes throughout).
- Manual smoke (optional, not CI): run the Example against a locally
  installed `codex` binary to confirm handshake and a trivial turn.

# Decisions That Constrain Ordering

- Phase 0 (transport) and phase 1 (types) are independent and can proceed in
  either order or in parallel.
- Phase 2 (client-init) needs both 0 and 1. Phases 3 -> 4 -> 5 build the
  thread/turn/approval stack sequentially on 2.
- Phase 6 (auth) depends only on 2 and can run in parallel with 3-5.
- Phase 7 closes the plan and depends on 5 and 6.

# Next Implementation Target

Build the JSON-RPC stdio transport with subprocess management, framing, id correlation, and dispatch, tested over in-memory pipes.
