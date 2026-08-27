---
description: "Port claude-agent-sdk-python to an idiomatic Go package internal/claude: types, options, subprocess transport, control protocol, query/client APIs, SDK MCP tools, errors, tests."
plan_status: in-progress
registered_at: "2026-08-27T20:07:58Z"
---
> NEXT: Implement the foundation: error types, message/content-block types, permission and hook types, and the Options struct with tests. ([Phase 0](phases/00-types-options-errors.md))

# Phases

- [x] [Phase 00: Errors, message types, and options](phases/00-types-options-errors.md)
- [x] [Phase 01: Message parser](phases/01-message-parser.md)
- [x] [Phase 02: Transport interface and subprocess CLI transport](phases/02-subprocess-transport.md)
- [x] [Phase 03: Control protocol engine](phases/03-control-protocol.md)
- [x] [Phase 04: One-shot Query API](phases/04-query-api.md)
- [x] [Phase 05: Bidirectional Client API](phases/05-client-api.md)
- [x] [Phase 06: In-process SDK MCP servers](phases/06-sdk-mcp.md)
- [ ] [Phase 07: Package docs, examples, and full-suite verification](phases/07-docs-and-verification.md)

# Shared Verification

- Per-phase: `go build ./...`, `go vet ./...`, and `go test
  ./internal/claude` from `cli/mohae` after each phase; each phase adds its
  own `_test.go` coverage as listed in its Done When.
- Fixtures for parser and protocol tests are taken from the vendored Python
  SDK's test suite (`ref/claude/claude-agent-sdk-python/tests/`) so wire
  compatibility is checked against the same data the reference implementation
  uses.
- Whole plan: phase 7's full-suite run plus the env-guarded live smoke test
  against an installed `claude` CLI when available.

# Decisions That Constrain Ordering

Phase 0 (types/options/errors) is the foundation. Phases 1 (parser) and 2
(transport) depend only on 0 and are independent of each other. Phase 3
(control protocol) needs both 1 and 2. Phases 4 (Query), 5 (Client), and 6
(SDK MCP) each depend only on 3 and are mutually independent. Phase 7 closes
the plan after 4, 5, and 6.

# Next Implementation Target

Implement the foundation: error types, message/content-block types, permission and hook types, and the Options struct with tests.
