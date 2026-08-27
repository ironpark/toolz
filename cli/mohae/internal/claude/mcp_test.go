package claude

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type addArgs struct {
	A float64 `json:"a"`
	B float64 `json:"b"`
}

var addSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"a": map[string]any{"type": "number"},
		"b": map[string]any{"type": "number"},
	},
	"required": []string{"a", "b"},
}

func calculatorServer(t *testing.T) *MCPServer {
	t.Helper()
	maxSize := 500000
	yes := true
	cfg := NewSDKMCPServer("calculator", "2.0.0",
		NewTool("add", "Add two numbers", addSchema,
			func(_ context.Context, args addArgs) (ToolResult, error) {
				return TextResult("Sum: %v", args.A+args.B), nil
			}),
		ToolDef{
			Name:        "boom",
			Description: "Always fails",
			Handler: func(context.Context, json.RawMessage) (ToolResult, error) {
				return ToolResult{}, errors.New("handler exploded")
			},
		},
		ToolDef{
			Name:        "panics",
			Description: "Panics",
			Handler: func(context.Context, json.RawMessage) (ToolResult, error) {
				panic("kaboom")
			},
		},
		ToolDef{
			Name:        "annotated",
			Description: "Has annotations",
			Annotations: &ToolAnnotations{ReadOnlyHint: &yes, MaxResultSizeChars: &maxSize},
			Handler: func(context.Context, json.RawMessage) (ToolResult, error) {
				return TextResult("ok"), nil
			},
		},
	)
	if cfg.Name != "calculator" {
		t.Fatalf("config = %+v", cfg)
	}
	return cfg.Instance
}

// rpc sends one JSON-RPC message to the server and decodes the reply.
func rpc(t *testing.T, s *MCPServer, message map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.handle(t.Context(), raw)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if out == nil {
		return nil
	}
	var reply map[string]any
	if err := json.Unmarshal(out, &reply); err != nil {
		t.Fatalf("reply %s is not JSON: %v", out, err)
	}
	return reply
}

func callTool(t *testing.T, s *MCPServer, name string, args map[string]any) map[string]any {
	t.Helper()
	reply := rpc(t, s, map[string]any{"jsonrpc": "2.0", "id": 9, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args}})
	result, ok := reply["result"].(map[string]any)
	if !ok {
		t.Fatalf("reply = %#v", reply)
	}
	return result
}

func resultText(t *testing.T, result map[string]any) string {
	t.Helper()
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		return ""
	}
	block, _ := content[0].(map[string]any)
	text, _ := block["text"].(string)
	return text
}

func TestMCPInitializeAndPing(t *testing.T) {
	t.Parallel()
	s := calculatorServer(t)

	reply := rpc(t, s, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-06-18"}})
	result := reply["result"].(map[string]any)
	// The client's protocol version is echoed rather than downgraded.
	if result["protocolVersion"] != "2025-06-18" {
		t.Fatalf("result = %#v", result)
	}
	info := result["serverInfo"].(map[string]any)
	if info["name"] != "calculator" || info["version"] != "2.0.0" {
		t.Fatalf("server info = %#v", info)
	}
	if _, ok := result["capabilities"].(map[string]any)["tools"]; !ok {
		t.Fatalf("capabilities = %#v", result["capabilities"])
	}

	// Without a version the default is offered.
	reply = rpc(t, s, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "initialize"})
	if reply["result"].(map[string]any)["protocolVersion"] != DefaultMCPProtocolVersion {
		t.Fatalf("result = %#v", reply["result"])
	}

	if reply = rpc(t, s, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "ping"}); reply["error"] != nil {
		t.Fatalf("ping = %#v", reply)
	}
}

func TestMCPNotificationsGetNoReply(t *testing.T) {
	t.Parallel()
	s := calculatorServer(t)
	if reply := rpc(t, s, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); reply != nil {
		t.Fatalf("notification reply = %#v", reply)
	}
	if reply := rpc(t, s, map[string]any{"jsonrpc": "2.0", "id": nil, "method": "notifications/cancelled"}); reply != nil {
		t.Fatalf("notification reply = %#v", reply)
	}
}

func TestMCPToolsList(t *testing.T) {
	t.Parallel()
	s := calculatorServer(t)
	reply := rpc(t, s, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	tools := reply["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 4 {
		t.Fatalf("tools = %#v", tools)
	}
	add := tools[0].(map[string]any)
	if add["name"] != "add" || add["description"] != "Add two numbers" {
		t.Fatalf("tool = %#v", add)
	}
	schema := add["inputSchema"].(map[string]any)
	if schema["type"] != "object" {
		t.Fatalf("schema = %#v", schema)
	}
	if _, ok := schema["properties"].(map[string]any)["a"]; !ok {
		t.Fatalf("schema = %#v", schema)
	}
	// A tool without a schema still advertises an object schema.
	boom := tools[1].(map[string]any)
	if boom["inputSchema"].(map[string]any)["type"] != "object" {
		t.Fatalf("tool = %#v", boom)
	}
	if _, ok := boom["annotations"]; ok {
		t.Fatalf("tool = %#v", boom)
	}
	// Annotations ride alongside, with maxResultSizeChars in _meta.
	annotated := tools[3].(map[string]any)
	if annotated["annotations"].(map[string]any)["readOnlyHint"] != true {
		t.Fatalf("annotations = %#v", annotated["annotations"])
	}
	meta := annotated["_meta"].(map[string]any)
	if meta["anthropic/maxResultSizeChars"] != float64(500000) {
		t.Fatalf("meta = %#v", meta)
	}
}

func TestMCPToolsCall(t *testing.T) {
	t.Parallel()
	s := calculatorServer(t)

	result := callTool(t, s, "add", map[string]any{"a": 2.0, "b": 3.0})
	if result["isError"] != false || resultText(t, result) != "Sum: 5" {
		t.Fatalf("result = %#v", result)
	}
}

func TestMCPToolsCallFailures(t *testing.T) {
	t.Parallel()
	s := calculatorServer(t)
	cases := []struct {
		name string
		tool string
		args map[string]any
		want string
	}{
		{"unknownTool", "nope", nil, "Tool 'nope' not found"},
		{"missingRequired", "add", map[string]any{"a": 1.0}, `Input validation error: "b" is a required property`},
		{"wrongType", "add", map[string]any{"a": "one", "b": 2.0}, `Input validation error: "a" must be of type number`},
		{"handlerError", "boom", nil, "handler exploded"},
		{"handlerPanic", "panics", nil, "Tool 'panics' panicked: kaboom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := callTool(t, s, tc.tool, tc.args)
			// Every failure is an isError result the model can read, never a
			// JSON-RPC error.
			if result["isError"] != true {
				t.Fatalf("result = %#v", result)
			}
			if got := resultText(t, result); got != tc.want {
				t.Fatalf("text = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMCPUnsupportedMethod(t *testing.T) {
	t.Parallel()
	s := calculatorServer(t)
	reply := rpc(t, s, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "resources/list"})
	rpcErr := reply["error"].(map[string]any)
	if rpcErr["code"] != float64(jsonRPCMethodNotFound) {
		t.Fatalf("error = %#v", rpcErr)
	}
	if reply["id"] != float64(1) {
		t.Fatalf("reply should echo the id: %#v", reply)
	}
}

func TestMCPToolContentConversion(t *testing.T) {
	t.Parallel()
	cfg := NewSDKMCPServer("content", "", ToolDef{
		Name: "mixed",
		Handler: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: []ToolContent{
				ToolText{Text: "hello"},
				ToolImage{Data: "AAAA", MimeType: "image/png"},
				ToolResourceLink{Name: "doc", URI: "file:///doc.md"},
				ToolResourceLink{},
				ToolResource{URI: "file:///a.txt", Text: "body"},
				// A binary resource has no text form and is dropped.
				ToolResource{URI: "file:///a.bin"},
			}}, nil
		},
	})
	result := callTool(t, cfg.Instance, "mixed", nil)
	content := result["content"].([]any)
	if len(content) != 5 {
		t.Fatalf("content = %#v", content)
	}
	if content[0].(map[string]any)["text"] != "hello" {
		t.Fatalf("content = %#v", content[0])
	}
	image := content[1].(map[string]any)
	if image["type"] != "image" || image["mimeType"] != "image/png" {
		t.Fatalf("content = %#v", image)
	}
	if content[2].(map[string]any)["text"] != "doc\nfile:///doc.md" {
		t.Fatalf("content = %#v", content[2])
	}
	if content[3].(map[string]any)["text"] != "Resource link" {
		t.Fatalf("content = %#v", content[3])
	}
	if content[4].(map[string]any)["text"] != "body" {
		t.Fatalf("content = %#v", content[4])
	}
}

func TestMCPMalformedMessage(t *testing.T) {
	t.Parallel()
	s := calculatorServer(t)
	out, err := s.handle(t.Context(), json.RawMessage(`not json`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	var reply map[string]any
	if err := json.Unmarshal(out, &reply); err != nil {
		t.Fatal(err)
	}
	if reply["error"].(map[string]any)["code"] != float64(jsonRPCInvalidRequest) {
		t.Fatalf("reply = %#v", reply)
	}
}

func TestSDKMCPServersFromOptions(t *testing.T) {
	t.Parallel()
	cfg := NewSDKMCPServer("calc", "")
	opts := &Options{MCPServers: map[string]MCPServerConfig{
		"calc":  cfg,
		"fs":    &MCPStdioServerConfig{Command: "node"},
		"empty": &MCPSDKServerConfig{Name: "empty"},
	}}
	servers := sdkMCPServers(opts)
	if len(servers) != 1 || servers["calc"] != cfg.Instance {
		t.Fatalf("servers = %#v", servers)
	}
}

func TestEngineRoutesMCPMessageToServer(t *testing.T) {
	t.Parallel()
	cfg := NewSDKMCPServer("calc", "", NewTool("add", "Add", addSchema,
		func(_ context.Context, args addArgs) (ToolResult, error) {
			return TextResult("Sum: %v", args.A+args.B), nil
		}))
	opts := &Options{MCPServers: map[string]MCPServerConfig{"calc": cfg}}

	ft := newFakeTransport()
	initResponder(ft, nil)
	opts.Transport = ft
	client := NewClient(opts)
	if err := client.Connect(t.Context()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Disconnect()

	ft.mu.Lock()
	ft.onWrite = nil
	ft.mu.Unlock()

	// A tools/call arriving over the control protocol reaches the handler.
	ft.push(map[string]any{"type": "control_request", "request_id": "m1", "request": map[string]any{
		"subtype": "mcp_message", "server_name": "calc",
		"message": map[string]any{"jsonrpc": "2.0", "id": 7, "method": "tools/call",
			"params": map[string]any{"name": "add", "arguments": map[string]any{"a": 20.0, "b": 22.0}}}}})
	out := ft.nextResponse(t)["response"].(map[string]any)
	rpcReply := out["mcp_response"].(map[string]any)
	if rpcReply["id"] != float64(7) {
		t.Fatalf("reply = %#v", rpcReply)
	}
	content := rpcReply["result"].(map[string]any)["content"].([]any)
	if content[0].(map[string]any)["text"] != "Sum: 42" {
		t.Fatalf("content = %#v", content)
	}

	// An unknown server is refused with a JSON-RPC error.
	ft.push(map[string]any{"type": "control_request", "request_id": "m2", "request": map[string]any{
		"subtype": "mcp_message", "server_name": "other",
		"message": map[string]any{"jsonrpc": "2.0", "id": 8, "method": "tools/list"}}})
	out = ft.nextResponse(t)["response"].(map[string]any)
	rpcErr := out["mcp_response"].(map[string]any)["error"].(map[string]any)
	if rpcErr["message"] != "Server 'other' not found" {
		t.Fatalf("error = %#v", rpcErr)
	}
}

func TestSDKMCPServerHoldsInputOpen(t *testing.T) {
	t.Parallel()
	cfg := NewSDKMCPServer("calc", "")
	ft := scriptedCLI(t, resultFrame())
	opts := &Options{
		Transport:  ft,
		MCPServers: map[string]MCPServerConfig{"calc": cfg},
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range Query(t.Context(), "hi", opts) {
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("query did not finish")
	}
	// An SDK MCP server is a bidirectional need, so the input stream is held
	// open until the run-ending result and then closed.
	if !ft.endedInput() {
		t.Fatal("input should have been ended")
	}
}
