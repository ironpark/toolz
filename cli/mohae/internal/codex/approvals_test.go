package codex

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// decisionOf decodes an approval reply.
func decisionOf(t *testing.T, reply *wireMessage) Decision {
	t.Helper()
	if reply.Error != nil {
		t.Fatalf("reply error: %+v", reply.Error)
	}
	var result decisionResult
	if err := json.Unmarshal(reply.Result, &result); err != nil {
		t.Fatalf("decode reply %s: %v", reply.Result, err)
	}
	return result.Decision
}

func TestCommandApprovalAccepted(t *testing.T) {
	requests := make(chan *CommandApprovalRequest, 1)
	client, server := connect(t, Options{
		Approvals: ApprovalFuncs{
			Command: func(ctx context.Context, req *CommandApprovalRequest) (Decision, error) {
				requests <- req
				return DecisionAcceptForSession, nil
			},
		},
	})
	threadID := startThread(t, client, server, "thr_1")
	stream := startTurn(t, client, server, threadID, "turn_1", Text("run tests"))

	server.notify(MethodItemStarted, map[string]any{"threadId": "thr_1", "turnId": "turn_1",
		"item": map[string]any{"type": "commandExecution", "id": "item_1",
			"command": "cargo test", "cwd": "/w", "status": "inProgress"}})

	replies := server.request("sr-1", MethodCommandApproval, map[string]any{
		"itemId": "item_1", "threadId": "thr_1", "turnId": "turn_1",
		"reason": "network access", "command": "cargo test", "cwd": "/w",
		"availableDecisions": []string{"accept", "decline"},
	})
	reply := server.awaitReply(replies)
	if got := decisionOf(t, reply); got != DecisionAcceptForSession {
		t.Fatalf("decision = %q", got)
	}

	select {
	case req := <-requests:
		if req.ItemID != "item_1" || req.ThreadID != "thr_1" || req.TurnID != "turn_1" {
			t.Fatalf("request = %+v", req)
		}
		if req.Command != "cargo test" || req.Cwd != "/w" || req.Reason != "network access" {
			t.Fatalf("request = %+v", req)
		}
		if len(req.AvailableDecisions) != 2 {
			t.Fatalf("availableDecisions = %v", req.AvailableDecisions)
		}
	case <-time.After(fakeTimeout):
		t.Fatal("handler never ran")
	}

	server.notify(MethodServerRequestResolved, map[string]any{"threadId": "thr_1", "requestId": "sr-1"})
	server.notify(MethodItemCompleted, map[string]any{"threadId": "thr_1", "turnId": "turn_1",
		"item": map[string]any{"type": "commandExecution", "id": "item_1",
			"command": "cargo test", "status": "completed", "exitCode": 0}})
	server.notify(MethodTurnCompleted, map[string]any{"threadId": "thr_1",
		"turn": map[string]any{"id": "turn_1", "status": "completed"}})

	final, err := stream.Wait(context.Background())
	if err != nil || final.Status != TurnCompleted {
		t.Fatalf("Wait = %+v, %v", final, err)
	}

	// serverRequest/resolved reaches the thread subscriber.
	var resolved bool
	for event := range client.ThreadEvents("thr_1") {
		if event.Method == MethodServerRequestResolved {
			resolved = true
			break
		}
	}
	if !resolved {
		t.Fatal("serverRequest/resolved not surfaced")
	}
}

func TestCommandApprovalDefaultDecline(t *testing.T) {
	_, server := connect(t, Options{})

	replies := server.request("sr-1", MethodCommandApproval, map[string]any{
		"itemId": "item_1", "threadId": "thr_1", "turnId": "turn_1", "command": "rm -rf /",
	})
	if got := decisionOf(t, server.awaitReply(replies)); got != DecisionDecline {
		t.Fatalf("decision = %q, want decline", got)
	}
}

func TestFileChangeApproval(t *testing.T) {
	seen := make(chan *FileChangeApprovalRequest, 1)
	_, server := connect(t, Options{
		Approvals: ApprovalFuncs{
			FileChange: func(ctx context.Context, req *FileChangeApprovalRequest) (Decision, error) {
				seen <- req
				return DecisionAccept, nil
			},
		},
	})

	replies := server.request("sr-2", MethodFileChangeApproval, map[string]any{
		"itemId": "item_2", "threadId": "thr_1", "turnId": "turn_1",
		"reason": "writes outside the workspace", "grantRoot": "/Users/me",
		"changes": []any{map[string]any{"path": "/a.go", "kind": "update", "diff": "@@"}},
	})
	if got := decisionOf(t, server.awaitReply(replies)); got != DecisionAccept {
		t.Fatalf("decision = %q", got)
	}
	req := <-seen
	if req.GrantRoot != "/Users/me" || len(req.Changes) != 1 || req.Changes[0].Path != "/a.go" {
		t.Fatalf("request = %+v", req)
	}
}

func TestFileChangeApprovalDefaultDecline(t *testing.T) {
	_, server := connect(t, Options{Approvals: ApprovalFuncs{}})

	replies := server.request("sr-3", MethodFileChangeApproval, map[string]any{
		"itemId": "item_2", "threadId": "thr_1", "turnId": "turn_1",
	})
	if got := decisionOf(t, server.awaitReply(replies)); got != DecisionDecline {
		t.Fatalf("decision = %q", got)
	}
}

func TestPermissionsApproval(t *testing.T) {
	_, server := connect(t, Options{
		Approvals: ApprovalFuncs{
			Permissions: func(ctx context.Context, req *PermissionsRequest) (*PermissionsResponse, error) {
				if req.Cwd != "/w" || req.EnvironmentID != "local" {
					t.Errorf("request = %+v", req)
				}
				return &PermissionsResponse{
					Permissions: json.RawMessage(`[{"type":"network"}]`),
					Scope:       ScopeSession,
				}, nil
			},
		},
	})

	replies := server.request("sr-4", MethodPermissionsApproval, map[string]any{
		"itemId": "item_3", "threadId": "thr_1", "turnId": "turn_1",
		"environmentId": "local", "cwd": "/w",
		"permissions": []any{map[string]any{"type": "network"}, map[string]any{"type": "filesystem"}},
	})
	reply := server.awaitReply(replies)
	var granted PermissionsResponse
	if err := json.Unmarshal(reply.Result, &granted); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if granted.Scope != ScopeSession || string(granted.Permissions) != `[{"type":"network"}]` {
		t.Fatalf("granted = %+v", granted)
	}
}

func TestPermissionsApprovalDefaultGrantsNothing(t *testing.T) {
	_, server := connect(t, Options{})

	replies := server.request("sr-5", MethodPermissionsApproval, map[string]any{
		"itemId": "item_3", "threadId": "thr_1", "turnId": "turn_1",
	})
	reply := server.awaitReply(replies)
	var granted PermissionsResponse
	if err := json.Unmarshal(reply.Result, &granted); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(granted.Permissions) != "[]" || granted.Scope != "" {
		t.Fatalf("granted = %+v", granted)
	}
}

func TestUserInputRequest(t *testing.T) {
	_, server := connect(t, Options{
		Approvals: ApprovalFuncs{
			UserInput: func(ctx context.Context, params json.RawMessage) (any, error) {
				return map[string]any{"answers": []string{"yes"}}, nil
			},
		},
	})

	replies := server.request("sr-6", MethodRequestUserInput, map[string]any{
		"threadId": "thr_1", "turnId": "turn_1", "questions": []any{},
	})
	reply := server.awaitReply(replies)
	if reply.Error != nil {
		t.Fatalf("error = %+v", reply.Error)
	}
	if string(reply.Result) != `{"answers":["yes"]}` {
		t.Fatalf("result = %s", reply.Result)
	}
}

func TestUserInputRequestDefaultErrors(t *testing.T) {
	_, server := connect(t, Options{})

	replies := server.request("sr-7", MethodRequestUserInput, map[string]any{"threadId": "thr_1"})
	reply := server.awaitReply(replies)
	if reply.Error == nil || reply.Error.Code != CodeMethodNotFound {
		t.Fatalf("error = %+v", reply.Error)
	}
}

func TestTokenRefreshRequest(t *testing.T) {
	_, server := connect(t, Options{
		Approvals: ApprovalFuncs{
			TokenRefresh: func(ctx context.Context, req *TokenRefreshRequest) (*ChatGPTAuthTokens, error) {
				if req.Reason != "unauthorized" || req.PreviousAccountID != "org-123" {
					t.Errorf("request = %+v", req)
				}
				return &ChatGPTAuthTokens{
					AccessToken: "jwt", ChatGPTAccountID: "org-123", ChatGPTPlanType: "business",
				}, nil
			},
		},
	})

	replies := server.request("sr-8", MethodChatGPTTokenRefresh, map[string]any{
		"reason": "unauthorized", "previousAccountId": "org-123",
	})
	reply := server.awaitReply(replies)
	var tokens ChatGPTAuthTokens
	if err := json.Unmarshal(reply.Result, &tokens); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if tokens.AccessToken != "jwt" || tokens.ChatGPTPlanType != "business" {
		t.Fatalf("tokens = %+v", tokens)
	}
}

func TestUnknownServerRequestGetsMethodNotFound(t *testing.T) {
	_, server := connect(t, Options{Approvals: ApprovalFuncs{}})

	replies := server.request("sr-9", "mcpServer/elicitation/request", map[string]any{"threadId": "thr_1"})
	reply := server.awaitReply(replies)
	if reply.Error == nil || reply.Error.Code != CodeMethodNotFound {
		t.Fatalf("error = %+v", reply.Error)
	}
}

func TestApprovalHandlerErrorBecomesRPCError(t *testing.T) {
	_, server := connect(t, Options{
		Approvals: ApprovalFuncs{
			Command: func(ctx context.Context, req *CommandApprovalRequest) (Decision, error) {
				return "", errors.New("ui unavailable")
			},
		},
	})

	replies := server.request("sr-10", MethodCommandApproval, map[string]any{"threadId": "thr_1"})
	reply := server.awaitReply(replies)
	if reply.Error == nil || reply.Error.Code != CodeInternalError {
		t.Fatalf("error = %+v", reply.Error)
	}
	if reply.Error.Message != "ui unavailable" {
		t.Fatalf("message = %q", reply.Error.Message)
	}
}

func TestApprovalContextCanceledOnTurnEnd(t *testing.T) {
	started := make(chan struct{})
	client, server := connect(t, Options{
		Approvals: ApprovalFuncs{
			Command: func(ctx context.Context, req *CommandApprovalRequest) (Decision, error) {
				close(started)
				<-ctx.Done()
				return DecisionCancel, nil
			},
		},
	})
	threadID := startThread(t, client, server, "thr_1")
	stream := startTurn(t, client, server, threadID, "turn_1", Text("hi"))

	replies := server.request("sr-11", MethodCommandApproval, map[string]any{
		"itemId": "item_1", "threadId": "thr_1", "turnId": "turn_1", "command": "sleep 100",
	})

	select {
	case <-started:
	case <-time.After(fakeTimeout):
		t.Fatal("handler never ran")
	}

	// Interrupting the turn clears the pending prompt.
	server.notify(MethodTurnCompleted, map[string]any{"threadId": "thr_1",
		"turn": map[string]any{"id": "turn_1", "status": "interrupted"}})

	if got := decisionOf(t, server.awaitReply(replies)); got != DecisionCancel {
		t.Fatalf("decision = %q", got)
	}
	final, err := stream.Wait(context.Background())
	if err != nil || final.Status != TurnInterrupted {
		t.Fatalf("Wait = %+v, %v", final, err)
	}
}

func TestApprovalContextCanceledOnClose(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	client, server := connect(t, Options{
		Approvals: ApprovalFuncs{
			FileChange: func(ctx context.Context, req *FileChangeApprovalRequest) (Decision, error) {
				close(started)
				<-ctx.Done()
				close(canceled)
				return DecisionDecline, nil
			},
		},
	})

	server.request("sr-12", MethodFileChangeApproval, map[string]any{
		"itemId": "item_1", "threadId": "thr_1", "turnId": "turn_1",
	})
	select {
	case <-started:
	case <-time.After(fakeTimeout):
		t.Fatal("handler never ran")
	}

	_ = client.Close()

	select {
	case <-canceled:
	case <-time.After(fakeTimeout):
		t.Fatal("handler context not canceled on Close")
	}
}
