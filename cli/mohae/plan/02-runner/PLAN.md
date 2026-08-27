---
description: Implement the mohae run trial runner and finish the init scaffolding
plan_status: in-progress
registered_at: "2026-08-27T21:46:21Z"
---
> NEXT: Finish the init scaffolding so the templates reference only files init writes. ([Phase 0](phases/00-init-scaffolding.md))

# Phases

- [ ] [Phase 00: Init scaffolding](phases/00-init-scaffolding.md)
- [ ] [Phase 01: Workspace preparation](phases/01-workspace.md)
- [ ] [Phase 02: Driver interface and custom-cli](phases/02-driver-custom.md)
- [ ] [Phase 03: claude-code and codex drivers with MCP](phases/03-driver-agents.md)
- [ ] [Phase 04: Trial runner](phases/04-trial.md)
- [ ] [Phase 05: Reports](phases/05-reports.md)
- [ ] [Phase 06: Run command wiring](phases/06-run-command.md)

# Shared Verification

Each phase ends with `go build ./... && go vet ./... && go test ./...` from
cli/mohae. Tests must pass with no network and no `claude` or `codex` binary
installed; shell stubs and the custom-cli driver stand in for a real agent.

# Decisions That Constrain Ordering

Phases run in order. Workspace preparation is what a driver needs a directory
for, drivers are what the trial loop calls, results are what the reports
render, and the run command is the last thing to wire together.

# Next Implementation Target

Finish the init scaffolding so the templates reference only files init writes.
