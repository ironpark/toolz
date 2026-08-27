---
depends_on:
- "00-internal-codex#5"
- "00-internal-codex#6"
perf_phase: false
status: in-progress
---
> DONE-WHEN: `go test -race ./internal/codex/...` passes; malformed-input and
> NEXT: none

# Fake-server hardening, docs, and package test sweep

## Planned Work

- Consolidate the test fake into a reusable `fakeserver_test.go` helper
  (script-driven: expect request -> respond, emit notification, send
  server-request) shared by all phase tests; remove duplication accumulated
  across phases.
- Robustness passes: oversized line handling, malformed JSON from server
  (log + skip vs fatal — decide and test), server error responses mapped to
  `*RPCError`, subprocess exit mid-turn failing active TurnStreams with a
  transport error, `-32001` overload error surfaced distinctly for caller
  retry.
- Race check: run `go test -race ./internal/codex/...` and fix findings.
- Doc comments on every exported identifier; package doc with a runnable-style
  usage example (client -> thread -> turn -> events -> close) as an
  `Example` test compiled but skipped without a real binary.
- Final sweep: `go build ./...`, `go vet ./...`, `gofmt -l` clean on the
  package.

## Done When

- `go test -race ./internal/codex/...` passes; malformed-input and
  process-death tests pass; `go vet` and `gofmt -l` report nothing; every
  exported identifier has a doc comment (spot-checked via `go doc`).
