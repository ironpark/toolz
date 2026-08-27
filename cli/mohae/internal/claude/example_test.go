package claude_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/ironpark/toolz/cli/mohae/internal/claude"
)

// These examples talk to a real `claude` CLI, so they are compiled but not run
// by `go test` (none declares an Output comment).

func ExampleQuery() {
	ctx := context.Background()
	for msg, err := range claude.Query(ctx, "What is 2+2?", nil) {
		if err != nil {
			log.Fatal(err)
		}
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Println(text.Text)
				}
			}
		case *claude.ResultMessage:
			if m.TotalCostUSD != nil {
				fmt.Printf("cost: $%.4f\n", *m.TotalCostUSD)
			}
		}
	}
}

func ExampleQuery_options() {
	maxTurns := 5
	opts := &claude.Options{
		SystemPrompt:   claude.SystemPromptText("You are an expert Go developer"),
		Cwd:            "/home/user/project",
		AllowedTools:   []string{"Read", "Grep"},
		PermissionMode: claude.PermissionModePlan,
		MaxTurns:       &maxTurns,
	}
	for msg, err := range claude.Query(context.Background(), "Plan a refactor", opts) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%T\n", msg)
	}
}

func ExampleClient() {
	ctx := context.Background()
	client := claude.NewClient(&claude.Options{PermissionMode: claude.PermissionModeAcceptEdits})
	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect()

	for _, prompt := range []string{"Describe this repo", "Now write a README"} {
		if err := client.Query(ctx, prompt, ""); err != nil {
			log.Fatal(err)
		}
		// ReceiveResponse stops after the turn's ResultMessage, leaving the
		// next turn's messages queued.
		for msg, err := range client.ReceiveResponse(ctx) {
			if err != nil {
				log.Fatal(err)
			}
			if result, ok := msg.(*claude.ResultMessage); ok {
				fmt.Println("turn finished:", result.Subtype)
			}
		}
	}
}

func ExampleClient_interrupt() {
	ctx := context.Background()
	client := claude.NewClient(nil)
	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect()

	if err := client.Query(ctx, "Count to a million", ""); err != nil {
		log.Fatal(err)
	}
	// Control calls are safe to make while another goroutine is receiving.
	if err := client.Interrupt(ctx); err != nil {
		log.Fatal(err)
	}
	if err := client.SetModel(ctx, "claude-sonnet-4-5"); err != nil {
		log.Fatal(err)
	}
}

func ExampleOptions_hooks() {
	opts := &claude.Options{
		Hooks: map[claude.HookEvent][]claude.HookMatcher{
			claude.HookPreToolUse: {{
				Matcher: "Bash",
				Hooks: []claude.HookCallback{
					func(_ context.Context, input map[string]any, toolUseID string, _ claude.HookContext) (claude.HookOutput, error) {
						command, _ := input["tool_input"].(map[string]any)["command"].(string)
						if command == "rm -rf /" {
							return claude.HookOutput{
								Decision: "block",
								Reason:   "refusing to delete the filesystem",
							}, nil
						}
						return claude.HookOutput{}, nil
					},
				},
			}},
		},
	}
	for _, err := range claude.Query(context.Background(), "Clean up temp files", opts) {
		if err != nil {
			log.Fatal(err)
		}
	}
}

func ExampleOptions_canUseTool() {
	opts := &claude.Options{
		CanUseTool: func(_ context.Context, toolName string, input map[string]any, permCtx claude.ToolPermissionContext) (claude.PermissionResult, error) {
			if toolName != "Write" {
				return &claude.PermissionResultDeny{Message: "only writes are allowed"}, nil
			}
			// Rewrite the input before allowing the call.
			updated := map[string]any{}
			for k, v := range input {
				updated[k] = v
			}
			updated["file_path"] = "/tmp/sandboxed.txt"
			fmt.Println("allowing", permCtx.ToolUseID)
			return &claude.PermissionResultAllow{UpdatedInput: updated}, nil
		},
	}
	for _, err := range claude.Query(context.Background(), "Write a haiku to a file", opts) {
		if err != nil {
			log.Fatal(err)
		}
	}
}

func ExampleNewSDKMCPServer() {
	type addArgs struct {
		A float64 `json:"a"`
		B float64 `json:"b"`
	}
	calculator := claude.NewSDKMCPServer("calculator", "1.0.0",
		claude.NewTool("add", "Add two numbers",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"a": map[string]any{"type": "number"},
					"b": map[string]any{"type": "number"},
				},
				"required": []string{"a", "b"},
			},
			func(_ context.Context, args addArgs) (claude.ToolResult, error) {
				return claude.TextResult("Sum: %v", args.A+args.B), nil
			}),
	)
	opts := &claude.Options{
		MCPServers:   map[string]claude.MCPServerConfig{"calc": calculator},
		AllowedTools: []string{"mcp__calc__add"},
	}
	for msg, err := range claude.Query(context.Background(), "What is 20 + 22?", opts) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Fprintf(os.Stdout, "%T\n", msg)
	}
}
