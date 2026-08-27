---
depends_on:
- "01-internal-claude#3"
perf_phase: false
status: in-progress
---
> DONE-WHEN: Tests: tools/list returns declared schemas and annotations meta;
> NEXT: none

# In-process SDK MCP servers

## Planned Work

- `mcp.go`: port the SDK MCP support from `__init__.py` +
  `_internal/sdk_mcp_bridge.py` without the Python `mcp` dependency:
  `ToolDef` struct (Name, Description, InputSchema as
  `map[string]any`/typed helper, Handler `func(ctx context.Context, args
  json.RawMessage) (ToolResult, error)`, optional Annotations incl.
  maxResultSizeChars via `_meta`), `NewTool` generic helper decoding args
  into a typed struct, `NewSDKMCPServer(name, version string, tools
  ...ToolDef) McpServerConfig`.
- JSON-RPC handling for the `mcp_message` control requests: `initialize`
  (protocol version/capabilities/serverInfo), `notifications/initialized`,
  `tools/list` (wire tool descriptors with inputSchema and annotations),
  `tools/call` (dispatch to handler; unknown tool, argument-decode failure,
  and handler error/panic all become `isError: true` tool results with text
  content, never JSON-RPC errors — matching the Python semantics);
  unsupported methods -> JSON-RPC method-not-found.
- Content conversion: text and image blocks pass through; resource links
  flatten to text; unsupported content dropped with a logged warning (port
  `_convert_tool_content`).
- Wire the server registry into the control engine's `mcp_message` handler
  (replacing the phase-3 stub) and into options serialization: sdk servers
  appear in `--mcp-config` by name/type only and register instances with the
  engine.

## Done When

- Tests: tools/list returns declared schemas and annotations meta;
  tools/call happy path, unknown tool, bad args, handler error and handler
  panic each produce the documented isError result; end-to-end via fake
  transport: a `control_request` mcp_message round-trips to a registered
  handler; `go test ./internal/claude` passes.
