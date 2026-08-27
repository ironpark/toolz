---
depends_on:
- "02-runner#5"
perf_phase: false
status: planned
---
> DONE-WHEN: An end-to-end test runs `mohae run` on a custom-cli configuration with a
> NEXT: none

# Run command wiring

## Planned Work

- Replace the `run` action's `notImplemented` with the runner: trials across
  every selected configuration, `--concurrency` in parallel, `--fail-fast`
  stopping at the first failure, `--show-dialogue` streaming the conversation.
- Return a non-zero exit status when any trial fails, and emit the reports in
  the formats requested by `--output` and the configuration.
- Update the existing tests that assert `run` is unimplemented, and update the
  Korean README where behaviour changed.

## Done When

- An end-to-end test runs `mohae run` on a custom-cli configuration with a
  shell-stub agent and asserts the reports, the exit status and the fail-fast
  and concurrency behaviour.
- `go build ./... && go vet ./... && go test ./...` pass.
