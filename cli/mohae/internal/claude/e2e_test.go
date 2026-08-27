package claude_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/toolz/cli/mohae/internal/claude"
)

// TestE2EQuery runs a trivial prompt against a real, installed CLI. It is
// skipped unless MOHAE_CLAUDE_E2E=1, since it costs money and needs
// credentials.
func TestE2EQuery(t *testing.T) {
	if os.Getenv("MOHAE_CLAUDE_E2E") != "1" {
		t.Skip("set MOHAE_CLAUDE_E2E=1 to run against a real claude CLI")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	opts := &claude.Options{
		SystemPrompt: claude.SystemPromptText("Answer with a single word."),
		Tools:        claude.ToolList{},
		Stderr:       func(line string) { t.Log("cli stderr:", line) },
	}

	var text strings.Builder
	var result *claude.ResultMessage
	for msg, err := range claude.Query(ctx, "What is the capital of France?", opts) {
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if block, ok := block.(*claude.TextBlock); ok {
					text.WriteString(block.Text)
				}
			}
		case *claude.ResultMessage:
			result = m
		}
	}
	if result == nil {
		t.Fatal("no result message")
	}
	if result.IsError {
		t.Fatalf("result reported an error: %+v", result)
	}
	if !strings.Contains(strings.ToLower(text.String()), "paris") {
		t.Fatalf("answer = %q", text.String())
	}
}

// TestE2EClient exercises a two-turn interactive session against a real CLI.
func TestE2EClient(t *testing.T) {
	if os.Getenv("MOHAE_CLAUDE_E2E") != "1" {
		t.Skip("set MOHAE_CLAUDE_E2E=1 to run against a real claude CLI")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	client := claude.NewClient(&claude.Options{Tools: claude.ToolList{}})
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Disconnect()

	if info := client.ServerInfo(); info == nil {
		t.Fatal("no server info after connect")
	}
	for _, prompt := range []string{"Remember the number 7.", "What number did I ask you to remember?"} {
		if err := client.Query(ctx, prompt, ""); err != nil {
			t.Fatalf("query: %v", err)
		}
		var sawResult bool
		for msg, err := range client.ReceiveResponse(ctx) {
			if err != nil {
				t.Fatalf("receive: %v", err)
			}
			if _, ok := msg.(*claude.ResultMessage); ok {
				sawResult = true
			}
		}
		if !sawResult {
			t.Fatal("turn ended without a result message")
		}
	}
}
