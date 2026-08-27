---
completed_at: "2026-08-27T20:35:52Z"
depends_on:
- "00-internal-codex#4"
perf_phase: false
status: done
---
> DONE-WHEN: A scripted turn requiring command approval completes when the handler
> NEXT: none

# Approvals and server-initiated requests

## Planned Work

- `approvals.go`: `ApprovalHandler` interface (or funcs on Options) with
  methods for `item/commandExecution/requestApproval` and
  `item/fileChange/requestApproval`, receiving typed request params
  (itemId, threadId, turnId, reason, command/cwd or changes/grantRoot) and a
  `context.Context`, returning a typed decision (`Accept`,
  `AcceptForSession`, `Decline`, `Cancel`); wire decisions to the transport
  server-request reply path.
- Handler for `item/permissions/requestApproval` returning the granted
  permission subset and scope; stub handlers for `tool/requestUserInput` and
  `account/chatgptAuthTokens/refresh` that return a documented default
  (decline / error) unless the caller overrides them.
- Default behavior with no handler registered: decline command/file
  approvals, error unknown server requests with JSON-RPC method-not-found so
  turns fail closed instead of hanging.
- Surface `serverRequest/resolved` notifications so UIs can clear pending
  prompts; clear pending handler contexts on turn completion/interrupt.
- `approvals_test.go`: fake server sends approval server-requests mid-turn;
  tests assert the handler's decision arrives as the JSON-RPC response,
  default-decline works, handler context is canceled when the turn is
  interrupted, and unknown server-request methods get an error reply.

## Done When

- A scripted turn requiring command approval completes when the handler
  accepts and ends declined when no handler is set; every server-request
  received during tests gets exactly one reply; interrupt cancels pending
  handler contexts.
