---
depends_on:
- "02-runner#4"
perf_phase: false
status: planned
---
> DONE-WHEN: Unit tests cover each renderer's content and the files written into the
> NEXT: none

# Reports

## Planned Work

- Render a set of trial results as terminal, json, markdown and html.
- Write the non-terminal formats into `report.dir`, named per run, and print
  the terminal rendering to stdout.
- Honour `--detailed-tokens` by breaking usage into input, output, cache read
  and cache write.

## Done When

- Unit tests cover each renderer's content and the files written into the
  report directory, including the detailed token breakdown.
