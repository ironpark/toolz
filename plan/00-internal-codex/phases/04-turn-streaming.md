---
completed_at: "2026-08-27T20:32:59Z"
depends_on:
- "00-internal-codex#3"
perf_phase: false
status: done
---
> DONE-WHEN: Scripted end-to-end test (init -> thread/start -> turn/start -> deltas ->
> NEXT: none

# Turn execution and event streaming

## Planned Work

- `turn.go`: `StartTurn(ctx, threadID, input, TurnOptions)` returning a
  `*TurnStream` handle; `SteerTurn(ctx, threadID, expectedTurnID, input)`;
  `InterruptTurn(ctx, threadID, turnID)`.
- `TurnStream`: `Events() <-chan Event` delivering typed events in arrival
  order (`TurnStarted`, `ItemStarted`, `ItemCompleted`, `AgentMessageDelta`,
  `ReasoningDelta`, `CommandOutputDelta`, `PlanUpdated`, `DiffUpdated`,
  `TokenUsageUpdated`, `TurnCompleted`); channel closes after
  `turn/completed` (any status) or client shutdown; `Wait(ctx)` convenience
  returning the final `Turn` with status
  completed/interrupted/failed and its error payload.
- Backpressure policy: bounded buffered channel; document that a slow consumer
  blocks the client-side dispatcher for that thread only (per-thread fan-out
  goroutine), never the transport reader.
- Route events by `threadId`/`turnId`; tolerate events for unknown turns
  (drop with debug log) so `thread/compact/start` and shell-command turns do
  not wedge the stream.
- `turn_test.go`: fake-server scripted event sequences asserting ordering,
  channel closure on each terminal status, interrupt flow ending in
  `interrupted`, steering an active turn, deltas accumulating to the final
  `agentMessage` item, and no goroutine leak when the consumer abandons the
  stream after context cancellation.

## Done When

- Scripted end-to-end test (init -> thread/start -> turn/start -> deltas ->
  turn/completed) passes through public API only; interrupt and failed-turn
  scripts produce the right terminal event; abandoning a stream does not
  block the reader (test with a never-read stream plus a second concurrent
  turn completing).
