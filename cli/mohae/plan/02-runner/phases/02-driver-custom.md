---
depends_on:
- "02-runner#1"
perf_phase: false
status: planned
---
> DONE-WHEN: Tests drive a shell-stub executable through the custom-cli driver, covering a
> NEXT: none

# Driver interface and custom-cli

## Planned Work

- Define the driver interface: a session opened on a workspace, a turn sent
  with a context, a response with text, usage and duration, and a close.
- Implement the custom-cli driver over os/exec: the configured command with the
  prompt on stdin, `agent.env` in the environment, the workspace as the working
  directory, and the turn context cancelling the process.
- Define the token usage type the drivers report into.

## Done When

- Tests drive a shell-stub executable through the custom-cli driver, covering a
  successful turn, a failing command, and a turn cancelled by its timeout.
