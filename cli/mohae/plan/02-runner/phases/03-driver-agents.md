---
depends_on:
- "02-runner#2"
perf_phase: false
status: planned
---
> DONE-WHEN: The MCP configuration parsing is unit tested against a config file, with no
> NEXT: none

# claude-code and codex drivers with MCP

## Planned Work

- Implement the claude-code driver on internal/claude and the codex driver on
  internal/codex, mapping model, effort, env, workspace and skills onto each.
- Parse the configured MCP server files with the modelcontextprotocol go-sdk
  types, and hand each driver the servers enabled for its agent type.
- Report per-turn text and token usage from each SDK's own result payload.

## Done When

- The MCP configuration parsing is unit tested against a config file, with no
  server ever launched.
- The drivers compile, are selected by agent type through the factory, and the
  factory's selection is tested without any agent binary present.
