---
depends_on:
- "01-internal-claude#0"
perf_phase: false
status: planned
---
> DONE-WHEN: Table-driven tests using real fixture lines (taken from the Python SDK's
> NEXT: none

# Message parser

## Planned Work

- `parser.go`: `parseMessage(data []byte|map) (Message, error)` porting
  `_internal/message_parser.py`: switch on `type` field (`user`, `assistant`,
  `system`, `result`, `stream_event`, task/notification kinds in scope);
  decode content-block arrays into the sealed ContentBlock types; preserve
  unknown block/message types by returning nil-with-no-error or a raw
  passthrough consistent with the Python parser (which skips unknowns);
  attach parent_tool_use_id, session_id, uuid and origin fields.
- Return `MessageParseError` with the offending payload on malformed
  required fields.

## Done When

- Table-driven tests using real fixture lines (taken from the Python SDK's
  tests / stream-json examples) cover every message kind, unknown-type
  skipping, and malformed-input errors; `go test ./internal/claude` passes.
