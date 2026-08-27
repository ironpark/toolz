package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// DefaultMCPProtocolVersion is offered when the client does not name one.
const DefaultMCPProtocolVersion = "2024-11-05"

// JSON-RPC error codes used by the in-process server.
const (
	jsonRPCInvalidRequest = -32600
	jsonRPCMethodNotFound = -32601
	jsonRPCInternalError  = -32603
)

// ---------------------------------------------------------------------------
// Tool results
// ---------------------------------------------------------------------------

// ToolContent is one block of a tool result. Implementations are ToolText,
// ToolImage, ToolResourceLink and ToolResource.
type ToolContent interface {
	// wire renders the block as MCP content, or reports false when the block
	// has no representation the CLI can render.
	wire() (map[string]any, bool)
}

// ToolText is a plain text result block.
type ToolText struct {
	Text string
}

func (t ToolText) wire() (map[string]any, bool) {
	return map[string]any{"type": "text", "text": t.Text}, true
}

// ToolImage is an image result block. Data is base64-encoded.
type ToolImage struct {
	Data     string
	MimeType string
}

func (i ToolImage) wire() (map[string]any, bool) {
	return map[string]any{"type": "image", "data": i.Data, "mimeType": i.MimeType}, true
}

// ToolResourceLink points at a resource. It is flattened to text, which is what
// the CLI renders.
type ToolResourceLink struct {
	Name        string
	URI         string
	Description string
}

func (l ToolResourceLink) wire() (map[string]any, bool) {
	parts := make([]string, 0, 3)
	for _, part := range []string{l.Name, l.URI, l.Description} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	text := "Resource link"
	if len(parts) > 0 {
		text = strings.Join(parts, "\n")
	}
	return map[string]any{"type": "text", "text": text}, true
}

// ToolResource embeds a resource. Text resources are flattened to text; a
// resource with no text (a binary blob) is dropped, since the CLI cannot render
// it.
type ToolResource struct {
	URI  string
	Text string
}

func (r ToolResource) wire() (map[string]any, bool) {
	if r.Text == "" {
		return nil, false
	}
	return map[string]any{"type": "text", "text": r.Text}, true
}

// ToolResult is what a tool handler returns.
type ToolResult struct {
	Content []ToolContent
	// IsError marks the result as a failure the model should read and react
	// to, as opposed to a protocol error.
	IsError bool
}

// TextResult is a ToolResult with a single text block.
func TextResult(format string, args ...any) ToolResult {
	if len(args) > 0 {
		format = fmt.Sprintf(format, args...)
	}
	return ToolResult{Content: []ToolContent{ToolText{Text: format}}}
}

// ErrorResult is a ToolResult reporting a failure to the model.
func ErrorResult(format string, args ...any) ToolResult {
	result := TextResult(format, args...)
	result.IsError = true
	return result
}

// wire renders the result as an MCP CallToolResult.
func (r ToolResult) wire() map[string]any {
	content := make([]map[string]any, 0, len(r.Content))
	for _, block := range r.Content {
		if encoded, ok := block.wire(); ok {
			content = append(content, encoded)
		}
	}
	return map[string]any{"content": content, "isError": r.IsError}
}

// ---------------------------------------------------------------------------
// Tool definitions
// ---------------------------------------------------------------------------

// ToolAnnotations are hints about a tool's behavior, plus MaxResultSizeChars.
type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
	// MaxResultSizeChars is the size up to which Claude Code keeps a result
	// inline rather than persisting it and showing a preview. It is not an
	// MCP hint: it travels in the tool's _meta, because MCP clients drop
	// annotation fields they do not know.
	MaxResultSizeChars *int `json:"-"`
}

// ToolHandler runs one tool call. args is the raw JSON arguments object.
// Returning an error produces an isError result, never a protocol error.
type ToolHandler func(ctx context.Context, args json.RawMessage) (ToolResult, error)

// ToolDef declares one tool of an in-process MCP server.
type ToolDef struct {
	Name        string
	Description string
	// InputSchema is the tool's JSON Schema. A nil schema means "an object
	// with no declared properties".
	InputSchema map[string]any
	Handler     ToolHandler
	Annotations *ToolAnnotations
}

// NewTool builds a ToolDef whose handler receives decoded arguments. Arguments
// that do not decode into T become an isError result, so a malformed call from
// the model is reported to it rather than failing the request.
func NewTool[T any](name, description string, schema map[string]any, handler func(ctx context.Context, args T) (ToolResult, error)) ToolDef {
	return ToolDef{
		Name:        name,
		Description: description,
		InputSchema: schema,
		Handler: func(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
			var args T
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &args); err != nil {
					return ErrorResult("Input validation error: %s", err), nil
				}
			}
			return handler(ctx, args)
		},
	}
}

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

// MCPServer is an in-process MCP server. Build one with NewSDKMCPServer and
// register it through Options.MCPServers; the SDK serves its JSON-RPC traffic
// over the control protocol, with no subprocess or socket in between.
type MCPServer struct {
	name    string
	version string
	tools   []ToolDef
	byName  map[string]ToolDef
}

// NewSDKMCPServer builds an in-process MCP server configuration. An empty
// version defaults to "1.0.0".
//
//	calc := claude.NewSDKMCPServer("calculator", "", claude.NewTool(
//		"add", "Add two numbers",
//		map[string]any{"type": "object", "properties": map[string]any{
//			"a": map[string]any{"type": "number"},
//			"b": map[string]any{"type": "number"}},
//			"required": []string{"a", "b"}},
//		func(ctx context.Context, args struct{ A, B float64 }) (claude.ToolResult, error) {
//			return claude.TextResult("Sum: %v", args.A+args.B), nil
//		}))
//	opts := &claude.Options{MCPServers: map[string]claude.MCPServerConfig{"calc": calc}}
func NewSDKMCPServer(name, version string, tools ...ToolDef) *MCPSDKServerConfig {
	if version == "" {
		version = "1.0.0"
	}
	server := &MCPServer{name: name, version: version, tools: tools, byName: map[string]ToolDef{}}
	for _, tool := range tools {
		server.byName[tool.Name] = tool
	}
	return &MCPSDKServerConfig{Name: name, Instance: server}
}

// jsonRPCRequest is one message from the CLI's MCP client.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// handle answers one JSON-RPC message. It returns nil for notifications, which
// get no reply.
func (s *MCPServer) handle(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	var req jsonRPCRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return s.errorReply(nil, jsonRPCInvalidRequest, "Invalid JSON-RPC message")
	}
	if len(req.ID) == 0 || string(req.ID) == "null" {
		// A notification: nothing to answer.
		return nil, nil
	}

	switch req.Method {
	case "initialize":
		return s.reply(req.ID, map[string]any{
			"protocolVersion": negotiateProtocolVersion(req.Params),
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.name, "version": s.version},
		})
	case "ping":
		return s.reply(req.ID, map[string]any{})
	case "tools/list":
		return s.reply(req.ID, map[string]any{"tools": s.toolDescriptors()})
	case "tools/call":
		return s.reply(req.ID, s.callTool(ctx, req.Params).wire())
	default:
		return s.errorReply(req.ID, jsonRPCMethodNotFound, "Method not found: "+req.Method)
	}
}

// toolDescriptors renders the tool list for tools/list.
func (s *MCPServer) toolDescriptors() []map[string]any {
	out := make([]map[string]any, 0, len(s.tools))
	for _, tool := range s.tools {
		schema := tool.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		descriptor := map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
			"inputSchema": schema,
		}
		if tool.Annotations != nil {
			descriptor["annotations"] = tool.Annotations
			if tool.Annotations.MaxResultSizeChars != nil {
				// Client-specific hints ride in _meta under namespaced keys
				// because MCP clients drop annotation fields they do not know.
				descriptor["_meta"] = map[string]any{
					"anthropic/maxResultSizeChars": *tool.Annotations.MaxResultSizeChars,
				}
			}
		}
		out = append(out, descriptor)
	}
	return out
}

// callTool dispatches one tools/call. Unknown tools, invalid arguments, handler
// errors and handler panics all become isError results the model can read,
// never protocol errors.
func (s *MCPServer) callTool(ctx context.Context, params json.RawMessage) (result ToolResult) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &call); err != nil {
			return ErrorResult("Input validation error: %s", err)
		}
	}
	tool, ok := s.byName[call.Name]
	if !ok {
		return ErrorResult("Tool '%s' not found", call.Name)
	}
	if err := validateAgainstSchema(tool.InputSchema, call.Arguments); err != nil {
		return ErrorResult("Input validation error: %s", err)
	}

	defer func() {
		if r := recover(); r != nil {
			result = ErrorResult("Tool '%s' panicked: %v", call.Name, r)
		}
	}()
	out, err := tool.Handler(ctx, call.Arguments)
	if err != nil {
		return ErrorResult("%s", err.Error())
	}
	return out
}

func (s *MCPServer) reply(id json.RawMessage, result any) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

func (s *MCPServer) errorReply(id json.RawMessage, code int, message string) (json.RawMessage, error) {
	var rawID any
	if len(id) > 0 {
		rawID = id
	}
	return json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      rawID,
		"error":   map[string]any{"code": code, "message": message},
	})
}

// negotiateProtocolVersion echoes the client's protocol version when it named
// one, so a newer CLI is not forced down to an older revision.
func negotiateProtocolVersion(params json.RawMessage) string {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(params) > 0 && json.Unmarshal(params, &p) == nil && p.ProtocolVersion != "" {
		return p.ProtocolVersion
	}
	return DefaultMCPProtocolVersion
}

// validateAgainstSchema performs the minimal JSON Schema checks the SDK makes
// without a schema library: the argument object decodes, every required
// property is present, and declared primitive types match. Anything else in the
// schema is left to the handler.
func validateAgainstSchema(schema map[string]any, raw json.RawMessage) error {
	if schema == nil {
		return nil
	}
	args := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return fmt.Errorf("arguments must be an object: %w", err)
		}
	}
	for _, name := range schemaRequired(schema) {
		if _, ok := args[name]; !ok {
			return fmt.Errorf("%q is a required property", name)
		}
	}
	properties, _ := schema["properties"].(map[string]any)
	for name, value := range args {
		spec, ok := properties[name].(map[string]any)
		if !ok {
			continue
		}
		want, ok := spec["type"].(string)
		if !ok {
			continue
		}
		if !matchesJSONType(want, value) {
			return fmt.Errorf("%q must be of type %s", name, want)
		}
	}
	return nil
}

func schemaRequired(schema map[string]any) []string {
	switch required := schema["required"].(type) {
	case []string:
		return required
	case []any:
		out := make([]string, 0, len(required))
		for _, item := range required {
			if name, ok := item.(string); ok {
				out = append(out, name)
			}
		}
		return out
	}
	return nil
}

func matchesJSONType(want string, value any) bool {
	switch want {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		f, ok := value.(float64)
		return ok && f == float64(int64(f))
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "null":
		return value == nil
	}
	return true
}

// ---------------------------------------------------------------------------
// Engine wiring
// ---------------------------------------------------------------------------

// attachSDKMCPServers wires the in-process MCP servers configured in opts into
// the engine's mcp_message routing. With none configured the router stays nil
// and the engine answers mcp_message requests with a JSON-RPC
// method-not-found error.
func attachSDKMCPServers(eng *engine, opts *Options) {
	servers := sdkMCPServers(opts)
	if len(servers) == 0 {
		return
	}
	eng.mcpRouter = func(ctx context.Context, serverName string, message json.RawMessage) (json.RawMessage, error) {
		server, ok := servers[serverName]
		if !ok {
			return nil, fmt.Errorf("Server '%s' not found", serverName)
		}
		return server.handle(ctx, message)
	}
}

// sdkMCPServers returns the in-process MCP servers configured on opts, keyed by
// the name they are registered under.
func sdkMCPServers(opts *Options) map[string]*MCPServer {
	var out map[string]*MCPServer
	for name, cfg := range opts.MCPServers {
		sdk, ok := cfg.(*MCPSDKServerConfig)
		if !ok {
			continue
		}
		if sdk.Instance == nil {
			continue
		}
		server := sdk.Instance
		if out == nil {
			out = map[string]*MCPServer{}
		}
		out[name] = server
	}
	return out
}
