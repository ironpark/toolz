# SCOPE

In scope: new files under `cli/mohae/internal/claude/` (and a
`claude/mcp`-style subpackage only if the SDK MCP server warrants it), plus
`go.mod`/`go.sum` updates for any new dependency (prefer stdlib +
`encoding/json` only). Out of scope: changes to existing mohae commands,
the `ref/` tree, session-store subsystem, and any network MCP client
implementation (stdio/SSE/HTTP MCP servers are passed through as CLI config
only, exactly as the Python SDK does).

# CONTEXT

## Current implementation and bottlenecks

`cli/mohae` is a small urfave/cli/v3 CLI (module
`github.com/ironpark/toolz/cli/mohae`, Go 1.26) with flat top-level files and
no `internal/` tree yet. There is no Go SDK for Claude Code; the only
reference is the vendored Python SDK. The Python SDK's architecture:

- Transport (`_internal/transport/subprocess_cli.py`): finds the `claude`
  binary (options.cli_path, PATH, common install locations), spawns it with
  `--output-format stream-json --input-format stream-json --verbose`, flags
  derived from options (`--system-prompt`, `--allowedTools`, `--mcp-config`,
  `--permission-mode`, `--resume`, `--model`, `--settings`, `--add-dir`,
  `--max-turns`, extra_args passthrough...), env `CLAUDE_CODE_ENTRYPOINT=sdk-py`
  plus options.env, cwd; reads newline-delimited JSON from stdout with a 1MB
  default max buffer (speculative json.loads on partial accumulation), streams
  stderr to a callback, and surfaces exit codes.
- Control protocol (`_internal/query.py` Query class): a reader loop
  demultiplexes stdout lines into (a) `control_request` messages from the CLI
  (can_use_tool, hook_callback, mcp_message) answered with
  `control_response`, (b) `control_response` replies matched to pending
  SDK-initiated `control_request`s by request id (initialize, interrupt,
  set_permission_mode, set_model, rewind_files, mcp server control,
  stop_task, mcp_status, context_usage), and (c) regular messages delivered
  to the consumer. Initialize exchanges hook registration and capabilities.
- Message parsing (`_internal/message_parser.py`): raw dict -> typed
  UserMessage/AssistantMessage (content blocks: text, thinking, tool_use,
  tool_result, server_tool_use, server_tool_result), SystemMessage,
  ResultMessage (usage, cost, errors), StreamEvent, task/notification
  messages.
- Public API: `query()` (one-shot async iterator; string or streaming
  prompt) and `ClaudeSDKClient` (connect, query, receive_messages,
  receive_response terminating at ResultMessage, interrupt, setters,
  context-manager lifecycle).
- SDK MCP (`__init__.py`, `_internal/sdk_mcp_bridge.py`): in-process MCP
  servers; `tool()` + `create_sdk_mcp_server()` build a server whose
  initialize/tools list/tools call requests arrive over the control
  protocol as `mcp_message` and are answered in-process; handler errors and
  schema-validation failures become isError tool results, never protocol
  errors.
- Errors (`_errors.py`): ClaudeSDKError base; CLINotFoundError,
  CLIConnectionError, ProcessError (exit code + stderr), CLIJSONDecodeError,
  MessageParseError, ResultError.

## Target structure and invariants

Package `github.com/ironpark/toolz/cli/mohae/internal/claude`, files split by
concern: `errors.go`, `types.go` (messages/content blocks), `options.go`,
`transport.go` (Transport interface), `subprocess.go` (CLI transport),
`parser.go`, `control.go` (protocol engine), `query.go` (one-shot API),
`client.go` (bidirectional API), `mcp.go` (SDK MCP server), plus `_test.go`
files. Invariants:

- Every blocking call takes a `context.Context`; cancellation kills the
  subprocess and unblocks readers.
- Message streams are exposed as `iter.Seq2[Message, error]` (Go 1.23+
  range-over-func) backed by channels internally; errors terminate the
  sequence rather than panic.
- Union types (Message, ContentBlock, PermissionResult, hook outputs) are
  sealed interfaces with concrete structs, decoded by a discriminator switch,
  not `map[string]any` passed to callers.
- Callbacks (CanUseTool, hooks, MCP tool handlers) are typed funcs receiving
  `context.Context`; goroutines handling control requests are tracked and
  joined on close; no goroutine leaks (verified in tests with fake
  transport).
- Errors are wrapped `errors.Is/As`-friendly sentinel/struct types mirroring
  the Python hierarchy.
- No new third-party dependency unless JSON Schema validation for SDK MCP
  tools justifies one; otherwise validation is minimal (required keys +
  primitive types) and documented.
