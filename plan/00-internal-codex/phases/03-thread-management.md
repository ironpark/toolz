---
depends_on:
- "00-internal-codex#2"
perf_phase: false
status: planned
---
> DONE-WHEN: Every listed method has a wire-shape + decode test against the fake server;
> NEXT: none

# Thread management

## Planned Work

- `thread.go`: client methods `StartThread(ctx, StartThreadParams)`,
  `ResumeThread`, `ForkThread`, `ReadThread` (with `includeTurns`),
  `ListThreads` (cursor pagination helper returning page + next cursor),
  `ArchiveThread`, `UnarchiveThread`, `DeleteThread`, `UnsubscribeThread`,
  `SetThreadName`.
- Params support model, cwd, approvalPolicy, sandbox policy, personality,
  and serviceName as documented; omit experimental fields.
- Track subscribed thread ids so incoming `thread/*` and `turn/*`
  notifications route to the right subscriber; expose thread lifecycle
  notifications (`thread/started`, `thread/status/changed`, `thread/archived`,
  `thread/closed`) via a per-client event channel or callback.
- `thread_test.go`: fake-server tests for each method verifying request wire
  shape (method name, params) and response decoding, plus pagination loop
  over two pages and notification routing to the right thread.

## Done When

- Every listed method has a wire-shape + decode test against the fake server;
  `ListThreads` pagination test walks `nextCursor` to exhaustion; thread
  notifications reach subscribers for their thread only.
