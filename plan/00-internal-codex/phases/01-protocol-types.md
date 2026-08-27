---
perf_phase: false
status: planned
---
> DONE-WHEN: All documented example payloads in scope decode into typed values without
> NEXT: none

# Protocol types

## Planned Work

- `types.go`: Go structs for the stable v2 surface used by this client:
  `Thread`, `Turn`, `TurnError`, `TokenUsage`, input items (`text`, `image`,
  `localImage`, `skill`), `SandboxPolicy` variants (`readOnly`,
  `workspaceWrite`, `dangerFullAccess`, `externalSandbox`) with read-access
  sub-structures, `ApprovalPolicy`, and request/response param structs for
  initialize, thread/*, turn/*, and account/* methods in scope.
- `ThreadItem` tagged union: `type`-discriminated custom
  `UnmarshalJSON` producing concrete item structs (`userMessage`,
  `agentMessage`, `reasoning`, `commandExecution`, `fileChange`,
  `mcpToolCall`, `webSearch`, `plan`, `enteredReviewMode`,
  `exitedReviewMode`, `contextCompaction`) plus a raw fallback for unknown
  types so new server versions do not break decoding.
- Notification payload types for `thread/started`, `thread/status/changed`,
  `turn/started`, `turn/completed`, `turn/diff/updated`, `turn/plan/updated`,
  `item/started`, `item/completed`, and the delta notifications
  (`item/agentMessage/delta`, `item/reasoning/*`,
  `item/commandExecution/outputDelta`).
- `errors.go`: `RPCError` (code/message/data) implementing `error`, turn
  failure info (`codexErrorInfo` values as string constants), and sentinel
  errors (`ErrClosed`, `ErrNotInitialized`).
- `types_test.go`: round-trip and golden decoding tests using JSON literals
  taken from ref/codex/app-server.md examples, including unknown-item
  fallback.

## Done When

- All documented example payloads in scope decode into typed values without
  data loss (verified by golden tests); unknown item and notification types
  decode into a raw variant rather than erroring; `go vet` clean.
