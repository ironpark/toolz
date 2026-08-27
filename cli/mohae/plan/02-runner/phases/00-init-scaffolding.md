---
completed_at: "2026-08-27T21:47:37Z"
perf_phase: false
status: done
---
> DONE-WHEN: `mohae init --all` writes a project that `mohae verify --check-scripts
> NEXT: none

# Init scaffolding

## Planned Work

- Write the remaining files the templates reference: PROMPT.md, the fixture
  workspace directory, and mcp.json for the mcp-server template.
- Add the flags that select them (`--with-prompt`, `--with-fixture`,
  `--with-mcp`, and `--all` for everything the chosen template needs).
- Keep the existing clobber check covering the new files.

## Done When

- `mohae init --all` writes a project that `mohae verify --check-scripts
  --check-agent-md` accepts, covered by a test.
- The existing init tests still pass unchanged in meaning.
