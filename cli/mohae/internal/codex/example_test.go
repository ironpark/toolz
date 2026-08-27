package codex_test

import (
	"context"
	"fmt"
	"log"

	"github.com/ironpark/toolz/cli/mohae/internal/codex"
)

// Example drives one turn end to end: spawn the app-server, start a thread,
// stream the agent's reply, and report the final status. It needs a real
// `codex` binary on PATH, so it is compiled but not run by `go test`.
func Example() {
	ctx := context.Background()

	client, err := codex.New(ctx, codex.Options{
		ClientInfo: codex.ClientInfo{Name: "mohae", Title: "Mohae", Version: "0.1.0"},
		Approvals: codex.ApprovalFuncs{
			Command: func(ctx context.Context, req *codex.CommandApprovalRequest) (codex.Decision, error) {
				log.Printf("approving %q in %s", req.Command, req.Cwd)
				return codex.DecisionAccept, nil
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	thread, err := client.StartThread(ctx, codex.StartThreadParams{
		Cwd:            "/Users/me/project",
		ApprovalPolicy: codex.ApprovalUnlessTrusted,
		SandboxPolicy:  codex.SandboxWorkspaceWrite([]string{"/Users/me/project"}, false, nil),
	})
	if err != nil {
		log.Fatal(err)
	}

	stream, err := client.StartTurn(ctx, thread.ID, codex.Text("Summarize this repo."), nil)
	if err != nil {
		log.Fatal(err)
	}
	for event := range stream.Events() {
		switch event.Kind {
		case codex.EventAgentMessageDelta:
			fmt.Print(event.Delta)
		case codex.EventCommandOutputDelta:
			fmt.Printf("[%s] %s", event.Stream, event.Delta)
		}
	}

	turn, err := stream.Wait(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("turn finished:", turn.Status)
}
