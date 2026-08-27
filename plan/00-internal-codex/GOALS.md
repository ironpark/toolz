# GOALS

## Problem and the end result from the user's point of view

mohae needs to drive the Codex agent programmatically. The `codex app-server`
subprocess speaks JSON-RPC 2.0 (without the `jsonrpc` header) over
newline-delimited JSON on stdio, with bidirectional traffic: client requests,
server responses, server-initiated requests (approvals, tool calls, token
refresh), and a stream of notifications (`thread/*`, `turn/*`, `item/*`).
The end result is a Go package `internal/codex` that spawns and manages the
subprocess, performs the initialize handshake, starts/resumes threads, runs
turns while streaming events to the caller through channels, and routes
approval requests to caller-supplied handlers — all idiomatic Go with
`context.Context` cancellation.

## Measurable goals

- `go build ./...` and `go vet ./...` pass with the new package.
- `go test ./internal/codex/...` passes; every exported entry point (transport
  dispatch, handshake, thread ops, turn streaming, approvals) is covered by
  unit tests against an in-process fake app-server (no real `codex` binary
  required in CI).
- A caller can: create a client, start a thread, run a turn with text input,
  receive ordered events (`turn/started`, `item/*` deltas, `turn/completed`),
  answer a command-execution approval, and interrupt a turn — each via public
  API only.

## Supported scope and non-goals

Supported: stdio transport, initialize/initialized handshake, thread
start/resume/fork/list/read/archive/delete, turn start/steer/interrupt with
event streaming, approvals (command execution, file change, permissions),
`account/read` and login flows needed to check auth state, typed errors.
Non-goals: WebSocket/unix-socket transports, experimental API surface
(`process/*`, dynamic tools, background terminals, plugins/marketplace/apps),
review mode, fs/* API, config editing RPCs, and any TUI. These can be added
later behind the same transport.

## Reference source / commit / license

Protocol reference: `cli/mohae/ref/codex/app-server.md` (official Codex
app-server documentation snapshot; upstream openai/codex, Apache-2.0). No
code is copied; only the wire protocol is implemented.

## Completion criteria for the whole plan

All phases done; `go build`, `go vet`, and `go test ./internal/codex/...`
green; package exposes the documented public API with doc comments; no changes
to existing mohae commands are required by this plan.
