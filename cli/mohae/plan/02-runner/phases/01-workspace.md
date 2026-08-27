---
depends_on:
- "02-runner#0"
perf_phase: false
status: planned
---
> DONE-WHEN: Unit tests cover the copy (modes, nesting, .git exclusion), the init script
> NEXT: none

# Workspace preparation

## Planned Work

- Copy `workspace.source` into an isolated per-trial directory, preserving file
  modes and symlinks, skipping .git.
- Run `init_script` inside the copy, install `agent_md` as AGENTS.md, install
  the skills enabled for the agent type, and `git init` plus an initial commit
  when `workspace.git` is set.
- Provide cleanup, and a place for the trial to keep its scratch directory.

## Done When

- Unit tests cover the copy (modes, nesting, .git exclusion), the init script
  running in the copy, AGENTS.md installation, skill scoping by agent type, and
  the source directory being left untouched.
