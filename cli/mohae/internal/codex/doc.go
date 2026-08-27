// Package codex drives the Codex agent through the `codex app-server`
// protocol.
//
// The app-server speaks JSON-RPC 2.0 (with the "jsonrpc" header omitted on the
// wire) over newline-delimited JSON on the subprocess stdio. Traffic is
// bidirectional: client requests and responses, server-initiated requests such
// as approval prompts, and a stream of thread, turn, and item notifications.
//
// # Lifecycle
//
// New spawns the subprocess and performs the initialize/initialized handshake.
// StartThread (or ResumeThread, ForkThread) opens a conversation and subscribes
// to its events. StartTurn sends user input and returns a TurnStream carrying
// typed events until the turn reaches a terminal status. Close stops the
// subprocess and releases every waiting caller.
//
//	client, err := codex.New(ctx, codex.Options{
//		ClientInfo: codex.ClientInfo{Name: "mohae", Version: "0.1.0"},
//		Approvals: codex.ApprovalFuncs{
//			Command: func(ctx context.Context, req *codex.CommandApprovalRequest) (codex.Decision, error) {
//				return codex.DecisionAccept, nil
//			},
//		},
//	})
//	if err != nil {
//		return err
//	}
//	defer client.Close()
//
//	thread, err := client.StartThread(ctx, codex.StartThreadParams{Cwd: "/repo"})
//	if err != nil {
//		return err
//	}
//	stream, err := client.StartTurn(ctx, thread.ID, codex.Text("Run the tests"), nil)
//	if err != nil {
//		return err
//	}
//	for event := range stream.Events() {
//		if event.Kind == codex.EventAgentMessageDelta {
//			fmt.Print(event.Delta)
//		}
//	}
//	turn, err := stream.Wait(ctx)
//
// # Delivery guarantees
//
// The transport reader never blocks. Turn events are handed to a per-thread
// pump that blocks only on its own thread's consumer, so a slow reader delays
// that thread alone. Thread lifecycle events and account updates use bounded
// channels whose entries are dropped when the consumer falls behind; turn
// events are never dropped while the stream is live. Abandoning a TurnStream
// with Close releases its pump immediately.
//
// # Failure modes
//
// Server error responses surface as *RPCError; use IsOverloaded to detect the
// retryable -32001 overload error. A failed turn carries a *TurnError whose
// Kind reports the codexErrorInfo discriminator. If the subprocess exits, the
// channel from Done closes, Err reports the cause, active turn streams fail
// with ErrClosed, and later calls return ErrClosed.
//
// Unknown item and notification types decode into raw fallbacks rather than
// failing, so a newer app-server does not break this client.
package codex
