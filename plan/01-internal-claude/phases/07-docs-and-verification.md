---
depends_on:
- "01-internal-claude#4"
- "01-internal-claude#5"
- "01-internal-claude#6"
perf_phase: false
status: in-progress
---
> DONE-WHEN: `go build ./...`, `go vet ./...`, `go test ./...` all pass from
> NEXT: none

# Package docs, examples, and full-suite verification

## Planned Work

- `doc.go` package comment documenting the public API surface, the mapping
  from the Python SDK names to the Go names, lifecycle rules (contexts,
  Disconnect, iterator break semantics), and what is intentionally not
  ported (session store subsystem).
- Runnable `Example*` test functions (compile-checked, guarded so they skip
  without a real CLI) for Query, Client multi-turn, a hook, a permission
  callback, and an SDK MCP tool.
- Optional smoke test behind an env guard (`MOHAE_CLAUDE_E2E=1`) that runs a
  trivial Query against a real installed CLI; skipped by default.
- Final sweep: `go build ./...`, `go vet ./...`, `go test ./...` from
  `cli/mohae`; fix anything the integration of phases surfaced; ensure no
  goroutine leaks across the suite.

## Done When

- `go build ./...`, `go vet ./...`, `go test ./...` all pass from
  `cli/mohae`; examples compile; package doc renders the full public API;
  plan phases 0-6 are all `done`.
