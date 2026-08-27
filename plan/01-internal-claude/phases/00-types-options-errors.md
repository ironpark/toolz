---
perf_phase: false
status: in-progress
---
> DONE-WHEN: `go build ./...` and `go vet ./...` pass; unit tests marshal/unmarshal
> NEXT: none

# Errors, message types, and options

## Planned Work

- Create `internal/claude` package with `errors.go`: `Error` base behavior via
  wrapping; concrete types `CLINotFoundError`, `CLIConnectionError`,
  `ProcessError{ExitCode int; Stderr string}`, `JSONDecodeError{Line string;
  Err error}`, `MessageParseError{Data json.RawMessage}`, `ResultError`
  (wraps a `ResultMessage`), all implementing `error` with `Unwrap` where
  applicable, ported from `_errors.py`.
- `types.go`: sealed `Message` interface (`UserMessage`, `AssistantMessage`,
  `SystemMessage`, `ResultMessage`, `StreamEvent`); sealed `ContentBlock`
  interface (`TextBlock`, `ThinkingBlock`, `ToolUseBlock`, `ToolResultBlock`,
  `ServerToolUseBlock`, `ServerToolResultBlock`); supporting structs
  (`ModelUsage`, cost/usage fields on `ResultMessage`, `MessageOrigin`,
  parent tool use IDs) ported from `types.py`, using `json.RawMessage` for
  open-ended payloads.
- Permission and hook types: `PermissionMode` constants (default,
  acceptEdits, plan, bypassPermissions, dontAsk, auto), `PermissionResult`
  (Allow with optional updated input / permission updates, Deny with message
  + interrupt), `PermissionUpdate`, `ToolPermissionContext`, `CanUseTool`
  func type; `HookEvent` constants, `HookMatcher`, `HookCallback` func type,
  `HookJSONOutput` struct covering the CLI wire fields (decision, block,
  systemMessage, hookSpecificOutput, async).
- `options.go`: `Options` struct mirroring `ClaudeAgentOptions` fields that
  are in scope (tools/allowed/disallowed, system prompt incl. preset variant,
  mcp_servers map, permission_mode, continue/resume/fork_session,
  session_id, max_turns, max_budget, model/fallback_model, betas, cwd,
  cli_path, settings, setting_sources, add_dirs, env, extra_args,
  max_buffer_size, stderr callback, can_use_tool, hooks, agents,
  include_partial_messages, user, thinking/effort/max_thinking_tokens,
  plugins, sandbox as raw JSON) with zero-value-friendly Go shapes and
  documented defaults; McpServerConfig variants (stdio/sse/http as
  passthrough config, sdk as in-process instance placeholder).

## Done When

- `go build ./...` and `go vet ./...` pass; unit tests marshal/unmarshal
  representative fixtures for each message and content-block type and for
  option JSON emission (e.g. mcp servers config), and `go test ./internal/claude`
  passes.
