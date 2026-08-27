---
depends_on:
- "02-runner#3"
perf_phase: false
status: planned
---
> DONE-WHEN: Tests run whole trials with the custom-cli driver: a passing trial, a failing
> NEXT: none

# Trial runner

## Planned Work

- Run one trial: prepare the workspace, open the driver, walk the prompts in
  order honouring `when`, `after` and per-turn timeouts under the trial-wide
  limit, record every turn, then run the verify commands outside the workspace
  with `$MOHAE_WORKSPACE` set.
- Build the trial result: per-turn responses, usage totals, durations, verify
  command outcomes and the overall verdict.

## Done When

- Tests run whole trials with the custom-cli driver: a passing trial, a failing
  verification, a skipped conditional prompt, a skipped dependent prompt, and a
  trial cut short by its timeout.
