package codex

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// serve runs fn on a goroutine so a blocking client call can be answered.
func serve(t *testing.T, fn func()) chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	return done
}

func TestStartThread(t *testing.T) {
	client, server := connect(t, Options{})

	done := serve(t, func() {
		req := server.expect("thread/start")
		var params StartThreadParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			t.Errorf("params: %v", err)
			return
		}
		if params.Model != "gpt-5.6-terra" || params.Cwd != "/Users/me/project" {
			t.Errorf("params = %+v", params)
		}
		if params.ApprovalPolicy != ApprovalNever || params.Sandbox != SandboxTypeWorkspaceWrite {
			t.Errorf("params = %+v", params)
		}
		if params.ServiceName != "mohae" {
			t.Errorf("serviceName = %q", params.ServiceName)
		}
		server.respond(req, map[string]any{"thread": map[string]any{
			"id": "thr_123", "sessionId": "thr_123", "modelProvider": "openai", "createdAt": 1730910000,
		}})
	})

	thread, err := client.StartThread(context.Background(), StartThreadParams{
		Model:          "gpt-5.6-terra",
		Cwd:            "/Users/me/project",
		ApprovalPolicy: ApprovalNever,
		Sandbox:        SandboxTypeWorkspaceWrite,
		Personality:    "friendly",
		ServiceName:    "mohae",
	})
	<-done
	if err != nil {
		t.Fatalf("StartThread: %v", err)
	}
	if thread.ID != "thr_123" || thread.SessionID != "thr_123" {
		t.Fatalf("thread = %+v", thread)
	}
	if client.ThreadEvents("thr_123") == nil {
		t.Fatal("thread/start did not subscribe")
	}
}

func TestResumeAndForkThread(t *testing.T) {
	client, server := connect(t, Options{})

	done := serve(t, func() {
		req := server.expect("thread/resume")
		var resume ResumeThreadParams
		if err := json.Unmarshal(req.Params, &resume); err != nil {
			t.Errorf("params: %v", err)
		}
		if resume.ThreadID != "thr_123" || resume.Personality != "friendly" {
			t.Errorf("resume params = %+v", resume)
		}
		server.respond(req, map[string]any{"thread": map[string]any{"id": "thr_123", "name": "Bug bash notes"}})

		req = server.expect("thread/fork")
		var fork ForkThreadParams
		if err := json.Unmarshal(req.Params, &fork); err != nil {
			t.Errorf("params: %v", err)
		}
		if fork.ThreadID != "thr_123" || fork.LastTurnID != "turn_456" {
			t.Errorf("fork params = %+v", fork)
		}
		server.respond(req, map[string]any{"thread": map[string]any{
			"id": "thr_456", "sessionId": "thr_123", "forkedFromId": "thr_123",
		}})
	})

	resumed, err := client.ResumeThread(context.Background(), ResumeThreadParams{
		ThreadID: "thr_123", Personality: "friendly",
	})
	if err != nil {
		t.Fatalf("ResumeThread: %v", err)
	}
	if resumed.Name == nil || *resumed.Name != "Bug bash notes" {
		t.Fatalf("name = %v", resumed.Name)
	}

	forked, err := client.ForkThread(context.Background(), ForkThreadParams{
		ThreadID: "thr_123", LastTurnID: "turn_456",
	})
	<-done
	if err != nil {
		t.Fatalf("ForkThread: %v", err)
	}
	if forked.ID != "thr_456" || forked.ForkedFromID != "thr_123" {
		t.Fatalf("forked = %+v", forked)
	}
	if client.ThreadEvents("thr_456") == nil {
		t.Fatal("fork did not subscribe")
	}
}

func TestReadThread(t *testing.T) {
	client, server := connect(t, Options{})

	done := serve(t, func() {
		req := server.expect("thread/read")
		var params ReadThreadParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			t.Errorf("params: %v", err)
		}
		if params.ThreadID != "thr_123" || !params.IncludeTurns {
			t.Errorf("params = %+v", params)
		}
		server.respond(req, map[string]any{"thread": map[string]any{
			"id": "thr_123", "name": "Bug bash notes", "ephemeral": false,
			"status": map[string]any{"type": "notLoaded"}, "turns": []any{},
		}})
	})

	thread, err := client.ReadThread(context.Background(), "thr_123", true)
	<-done
	if err != nil {
		t.Fatalf("ReadThread: %v", err)
	}
	if thread.Status == nil || thread.Status.Type != ThreadStatusNotLoaded {
		t.Fatalf("status = %+v", thread.Status)
	}
	// thread/read must not subscribe.
	if client.ThreadEvents("thr_123") != nil {
		t.Fatal("thread/read subscribed to the thread")
	}
}

func TestListThreadsPagination(t *testing.T) {
	client, server := connect(t, Options{})

	done := serve(t, func() {
		first := server.expect("thread/list")
		var params ListThreadsParams
		if err := json.Unmarshal(first.Params, &params); err != nil {
			t.Errorf("params: %v", err)
		}
		if params.Cursor != "" || params.Limit != 2 || params.SortKey != SortKeyCreatedAt {
			t.Errorf("first page params = %+v", params)
		}
		server.respond(first, map[string]any{
			"data":       []any{map[string]any{"id": "thr_a"}, map[string]any{"id": "thr_b"}},
			"nextCursor": "page-2",
		})

		second := server.expect("thread/list")
		if err := json.Unmarshal(second.Params, &params); err != nil {
			t.Errorf("params: %v", err)
		}
		if params.Cursor != "page-2" {
			t.Errorf("second page cursor = %q", params.Cursor)
		}
		server.respond(second, map[string]any{
			"data":       []any{map[string]any{"id": "thr_c"}},
			"nextCursor": nil,
		})
	})

	var ids []string
	for thread, err := range client.AllThreads(context.Background(), ListThreadsParams{
		Limit: 2, SortKey: SortKeyCreatedAt,
	}) {
		if err != nil {
			t.Fatalf("AllThreads: %v", err)
		}
		ids = append(ids, thread.ID)
	}
	<-done

	if len(ids) != 3 || ids[0] != "thr_a" || ids[2] != "thr_c" {
		t.Fatalf("ids = %v", ids)
	}
}

func TestListThreadsIterationStop(t *testing.T) {
	client, server := connect(t, Options{})

	done := serve(t, func() {
		req := server.expect("thread/list")
		server.respond(req, map[string]any{
			"data":       []any{map[string]any{"id": "thr_a"}, map[string]any{"id": "thr_b"}},
			"nextCursor": "page-2",
		})
	})

	count := 0
	for range client.AllThreads(context.Background(), ListThreadsParams{}) {
		count++
		break
	}
	<-done
	if count != 1 {
		t.Fatalf("count = %d", count)
	}
}

func TestThreadMutations(t *testing.T) {
	client, server := connect(t, Options{})

	tests := []struct {
		name       string
		method     string
		result     any
		invoke     func() error
		wantParams string
	}{
		{
			name:       "archive",
			method:     "thread/archive",
			result:     map[string]any{},
			invoke:     func() error { return client.ArchiveThread(context.Background(), "thr_b") },
			wantParams: `{"threadId":"thr_b"}`,
		},
		{
			name:       "delete",
			method:     "thread/delete",
			result:     map[string]any{},
			invoke:     func() error { return client.DeleteThread(context.Background(), "thr_b") },
			wantParams: `{"threadId":"thr_b"}`,
		},
		{
			name:   "unarchive",
			method: "thread/unarchive",
			result: map[string]any{"thread": map[string]any{"id": "thr_b", "name": "Bug bash notes"}},
			invoke: func() error {
				thread, err := client.UnarchiveThread(context.Background(), "thr_b")
				if err == nil && thread.ID != "thr_b" {
					t.Errorf("thread = %+v", thread)
				}
				return err
			},
			wantParams: `{"threadId":"thr_b"}`,
		},
		{
			name:   "name/set",
			method: "thread/name/set",
			result: map[string]any{},
			invoke: func() error {
				return client.SetThreadName(context.Background(), "thr_b", "Renamed")
			},
			wantParams: `{"threadId":"thr_b","name":"Renamed"}`,
		},
		{
			name:   "loaded/list",
			method: "thread/loaded/list",
			result: map[string]any{"data": []string{"thr_123", "thr_456"}},
			invoke: func() error {
				ids, err := client.ListLoadedThreads(context.Background())
				if err == nil && len(ids) != 2 {
					t.Errorf("ids = %v", ids)
				}
				return err
			},
			wantParams: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			done := serve(t, func() {
				req := server.expect(tc.method)
				if string(req.Params) != tc.wantParams {
					t.Errorf("params = %s, want %s", req.Params, tc.wantParams)
				}
				server.respond(req, tc.result)
			})
			if err := tc.invoke(); err != nil {
				t.Fatalf("%s: %v", tc.method, err)
			}
			<-done
		})
	}
}

func TestUnsubscribeThread(t *testing.T) {
	client, server := connect(t, Options{})

	done := serve(t, func() {
		req := server.expect("thread/start")
		server.respond(req, map[string]any{"thread": map[string]any{"id": "thr_1"}})
		req = server.expect("thread/unsubscribe")
		server.respond(req, map[string]any{"status": "unsubscribed"})
	})

	if _, err := client.StartThread(context.Background(), StartThreadParams{}); err != nil {
		t.Fatalf("StartThread: %v", err)
	}
	events := client.ThreadEvents("thr_1")
	if events == nil {
		t.Fatal("no subscription")
	}

	status, err := client.UnsubscribeThread(context.Background(), "thr_1")
	<-done
	if err != nil {
		t.Fatalf("UnsubscribeThread: %v", err)
	}
	if status != "unsubscribed" {
		t.Fatalf("status = %q", status)
	}
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("event channel still open")
		}
	case <-time.After(fakeTimeout):
		t.Fatal("event channel not closed by unsubscribe")
	}
	if client.ThreadEvents("thr_1") != nil {
		t.Fatal("subscription not removed")
	}
}

func TestThreadNotificationRouting(t *testing.T) {
	client, server := connect(t, Options{})

	done := serve(t, func() {
		for _, id := range []string{"thr_1", "thr_2"} {
			req := server.expect("thread/start")
			server.respond(req, map[string]any{"thread": map[string]any{"id": id}})
		}
	})
	for range 2 {
		if _, err := client.StartThread(context.Background(), StartThreadParams{}); err != nil {
			t.Fatalf("StartThread: %v", err)
		}
	}
	<-done

	first := client.ThreadEvents("thr_1")
	second := client.ThreadEvents("thr_2")

	server.notify(MethodThreadStatusChanged, map[string]any{
		"threadId": "thr_1",
		"status":   map[string]any{"type": "active", "activeFlags": []string{"waitingOnApproval"}},
	})
	server.notify(MethodThreadArchived, map[string]any{"threadId": "thr_1"})
	server.notify(MethodThreadNameUpdated, map[string]any{"threadId": "thr_1", "name": "Renamed"})
	// An event for a thread nobody subscribed to must be dropped silently.
	server.notify(MethodThreadClosed, map[string]any{"threadId": "thr_unknown"})

	statusEvent := recvThreadEvent(t, first)
	if statusEvent.Method != MethodThreadStatusChanged || statusEvent.Status == nil {
		t.Fatalf("event = %+v", statusEvent)
	}
	if statusEvent.Status.Type != ThreadStatusActive || len(statusEvent.Status.ActiveFlags) != 1 {
		t.Fatalf("status = %+v", statusEvent.Status)
	}
	if got := recvThreadEvent(t, first); got.Method != MethodThreadArchived {
		t.Fatalf("event = %+v", got)
	}
	if got := recvThreadEvent(t, first); got.Name != "Renamed" {
		t.Fatalf("event = %+v", got)
	}

	select {
	case event := <-second:
		t.Fatalf("thr_2 received %+v", event)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestThreadStartedNotificationCarriesThread(t *testing.T) {
	client, server := connect(t, Options{})

	done := serve(t, func() {
		req := server.expect("thread/start")
		server.respond(req, map[string]any{"thread": map[string]any{"id": "thr_1"}})
	})
	if _, err := client.StartThread(context.Background(), StartThreadParams{}); err != nil {
		t.Fatalf("StartThread: %v", err)
	}
	<-done

	server.notify(MethodThreadStarted, map[string]any{"thread": map[string]any{"id": "thr_1", "preview": "hello"}})

	event := recvThreadEvent(t, client.ThreadEvents("thr_1"))
	if event.Thread == nil || event.Thread.Preview != "hello" {
		t.Fatalf("event = %+v", event)
	}
	if event.ThreadID != "thr_1" {
		t.Fatalf("threadId = %q", event.ThreadID)
	}
}

func TestThreadEventsClosedOnShutdown(t *testing.T) {
	client, server := connect(t, Options{})

	done := serve(t, func() {
		req := server.expect("thread/start")
		server.respond(req, map[string]any{"thread": map[string]any{"id": "thr_1"}})
	})
	if _, err := client.StartThread(context.Background(), StartThreadParams{}); err != nil {
		t.Fatalf("StartThread: %v", err)
	}
	<-done

	events := client.ThreadEvents("thr_1")
	_ = client.Close()

	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("channel still open after Close")
		}
	case <-time.After(fakeTimeout):
		t.Fatal("channel not closed after Close")
	}
}

func recvThreadEvent(t *testing.T, ch <-chan ThreadEvent) ThreadEvent {
	t.Helper()
	select {
	case event, ok := <-ch:
		if !ok {
			t.Fatal("thread event channel closed")
		}
		return event
	case <-time.After(fakeTimeout):
		t.Fatal("timed out waiting for a thread event")
		return ThreadEvent{}
	}
}
