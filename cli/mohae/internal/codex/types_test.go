package codex

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestDecodeThreadStartResult(t *testing.T) {
	const payload = `{
	  "thread": {
	    "id": "thr_123",
	    "sessionId": "thr_123",
	    "preview": "",
	    "ephemeral": false,
	    "modelProvider": "openai",
	    "createdAt": 1730910000
	  }
	}`

	var res ThreadResult
	if err := json.Unmarshal([]byte(payload), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Thread.ID != "thr_123" || res.Thread.SessionID != "thr_123" {
		t.Fatalf("thread = %+v", res.Thread)
	}
	if res.Thread.ModelProvider != "openai" || res.Thread.CreatedAt != 1730910000 {
		t.Fatalf("thread = %+v", res.Thread)
	}
	if res.Thread.Ephemeral {
		t.Fatal("ephemeral should be false")
	}
}

func TestDecodeThreadListResult(t *testing.T) {
	const payload = `{
	  "data": [
	    { "id": "thr_a", "preview": "Create a TUI", "ephemeral": false, "isPinned": true, "modelProvider": "openai", "createdAt": 1730831111, "updatedAt": 1730831111, "name": "TUI prototype", "status": { "type": "notLoaded" } },
	    { "id": "thr_b", "preview": "Fix tests", "ephemeral": false, "isPinned": false, "modelProvider": "openai", "createdAt": 1730750000, "updatedAt": 1730750000, "status": { "type": "notLoaded" } }
	  ],
	  "nextCursor": "opaque-token-or-null"
	}`

	var res ListThreadsResult
	if err := json.Unmarshal([]byte(payload), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(res.Data) != 2 {
		t.Fatalf("data len = %d", len(res.Data))
	}
	if res.Data[0].Name == nil || *res.Data[0].Name != "TUI prototype" {
		t.Fatalf("name = %v", res.Data[0].Name)
	}
	if !res.Data[0].IsPinned || res.Data[1].IsPinned {
		t.Fatal("isPinned mismatch")
	}
	if res.Data[1].Status == nil || res.Data[1].Status.Type != ThreadStatusNotLoaded {
		t.Fatalf("status = %+v", res.Data[1].Status)
	}
	if res.NextCursor != "opaque-token-or-null" {
		t.Fatalf("cursor = %q", res.NextCursor)
	}
}

func TestDecodeThreadStatusChanged(t *testing.T) {
	const payload = `{"threadId":"thr_123","status":{"type":"active","activeFlags":["waitingOnApproval"]}}`

	var params ThreadStatusChangedParams
	if err := json.Unmarshal([]byte(payload), &params); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if params.ThreadID != "thr_123" || params.Status.Type != ThreadStatusActive {
		t.Fatalf("params = %+v", params)
	}
	if len(params.Status.ActiveFlags) != 1 || params.Status.ActiveFlags[0] != "waitingOnApproval" {
		t.Fatalf("activeFlags = %v", params.Status.ActiveFlags)
	}
}

func TestDecodeTurnStartResult(t *testing.T) {
	const payload = `{"turn":{"id":"turn_456","status":"inProgress","items":[],"error":null}}`

	var res StartTurnResult
	if err := json.Unmarshal([]byte(payload), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Turn.ID != "turn_456" || res.Turn.Status != TurnInProgress {
		t.Fatalf("turn = %+v", res.Turn)
	}
	if res.Turn.IsTerminal() {
		t.Fatal("inProgress turn reported terminal")
	}
	if res.Turn.Error != nil {
		t.Fatalf("error = %+v", res.Turn.Error)
	}
}

func TestDecodeFailedTurn(t *testing.T) {
	const payload = `{"turn":{"id":"turn_9","status":"failed","items":[],"error":{
	  "message":"upstream failed",
	  "codexErrorInfo":{"type":"HttpConnectionFailed","httpStatusCode":503},
	  "additionalDetails":{"attempts":3}
	}}}`

	var res StartTurnResult
	if err := json.Unmarshal([]byte(payload), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !res.Turn.IsTerminal() {
		t.Fatal("failed turn should be terminal")
	}
	turnErr := res.Turn.Error
	if turnErr == nil {
		t.Fatal("error missing")
	}
	if turnErr.Kind() != ErrorInfoHTTPConnectionFailed {
		t.Fatalf("kind = %q", turnErr.Kind())
	}
	status, ok := turnErr.HTTPStatusCode()
	if !ok || status != 503 {
		t.Fatalf("status = %d %v", status, ok)
	}
	if turnErr.Error() == "" {
		t.Fatal("empty error string")
	}
}

func TestDecodeTurnErrorInfoAsString(t *testing.T) {
	var turnErr TurnError
	if err := json.Unmarshal([]byte(`{"message":"nope","codexErrorInfo":"UsageLimitExceeded"}`), &turnErr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if turnErr.Kind() != ErrorInfoUsageLimitExceeded {
		t.Fatalf("kind = %q", turnErr.Kind())
	}
	if _, ok := turnErr.HTTPStatusCode(); ok {
		t.Fatal("unexpected http status")
	}
}

func TestDecodeThreadItems(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		check   func(*testing.T, ThreadItem)
	}{
		{
			name:    "agentMessage",
			payload: `{"type":"agentMessage","id":"item_1","text":"hello","phase":"final_answer"}`,
			check: func(t *testing.T, item ThreadItem) {
				msg, ok := item.Item.(*AgentMessageItem)
				if !ok {
					t.Fatalf("item = %T", item.Item)
				}
				if msg.Text != "hello" || msg.Phase != "final_answer" {
					t.Fatalf("msg = %+v", msg)
				}
			},
		},
		{
			name:    "userMessage",
			payload: `{"type":"userMessage","id":"turn_900","content":[{"type":"text","text":"Review commit"}]}`,
			check: func(t *testing.T, item ThreadItem) {
				msg := item.Item.(*UserMessageItem)
				if len(msg.Content) != 1 {
					t.Fatalf("content = %v", msg.Content)
				}
			},
		},
		{
			name:    "commandExecution",
			payload: `{"type":"commandExecution","id":"item_2","command":"ls -la","cwd":"/tmp","status":"completed","aggregatedOutput":"out","exitCode":0,"durationMs":12}`,
			check: func(t *testing.T, item ThreadItem) {
				cmd := item.Item.(*CommandExecutionItem)
				if cmd.Command != "ls -la" || cmd.Cwd != "/tmp" || cmd.Status != ItemStatusCompleted {
					t.Fatalf("cmd = %+v", cmd)
				}
				if cmd.ExitCode == nil || *cmd.ExitCode != 0 {
					t.Fatalf("exitCode = %v", cmd.ExitCode)
				}
				if cmd.DurationMs == nil || *cmd.DurationMs != 12 {
					t.Fatalf("durationMs = %v", cmd.DurationMs)
				}
			},
		},
		{
			name:    "fileChange",
			payload: `{"type":"fileChange","id":"item_3","status":"inProgress","changes":[{"path":"/a.go","kind":"update","diff":"@@"}]}`,
			check: func(t *testing.T, item ThreadItem) {
				fc := item.Item.(*FileChangeItem)
				if len(fc.Changes) != 1 || fc.Changes[0].Path != "/a.go" || fc.Changes[0].Kind != "update" {
					t.Fatalf("changes = %+v", fc.Changes)
				}
			},
		},
		{
			name:    "mcpToolCall",
			payload: `{"type":"mcpToolCall","id":"item_4","server":"github","tool":"search","status":"completed","arguments":{"q":"x"},"result":{"ok":true}}`,
			check: func(t *testing.T, item ThreadItem) {
				call := item.Item.(*McpToolCallItem)
				if call.Server != "github" || call.Tool != "search" {
					t.Fatalf("call = %+v", call)
				}
				if string(call.Arguments) != `{"q":"x"}` {
					t.Fatalf("arguments = %s", call.Arguments)
				}
			},
		},
		{
			name:    "webSearch",
			payload: `{"type":"webSearch","id":"item_5","query":"golang","action":{"type":"openPage","url":"https://go.dev"}}`,
			check: func(t *testing.T, item ThreadItem) {
				ws := item.Item.(*WebSearchItem)
				if ws.Action == nil || ws.Action.Type != "openPage" || ws.Action.URL != "https://go.dev" {
					t.Fatalf("action = %+v", ws.Action)
				}
			},
		},
		{
			name:    "reasoning",
			payload: `{"type":"reasoning","id":"item_6","summary":["a","b"],"content":["raw"]}`,
			check: func(t *testing.T, item ThreadItem) {
				r := item.Item.(*ReasoningItem)
				if len(r.Summary) != 2 || len(r.Content) != 1 {
					t.Fatalf("reasoning = %+v", r)
				}
			},
		},
		{
			name:    "plan",
			payload: `{"type":"plan","id":"item_7","text":"step one"}`,
			check: func(t *testing.T, item ThreadItem) {
				if item.Item.(*PlanItem).Text != "step one" {
					t.Fatal("plan text")
				}
			},
		},
		{
			name:    "enteredReviewMode",
			payload: `{"type":"enteredReviewMode","id":"turn_900","review":"current changes"}`,
			check: func(t *testing.T, item ThreadItem) {
				if item.Item.(*EnteredReviewModeItem).Review != "current changes" {
					t.Fatal("review")
				}
			},
		},
		{
			name:    "exitedReviewMode",
			payload: `{"type":"exitedReviewMode","id":"turn_900","review":"Looks solid overall..."}`,
			check: func(t *testing.T, item ThreadItem) {
				if item.Item.(*ExitedReviewModeItem).Review == "" {
					t.Fatal("review")
				}
			},
		},
		{
			name:    "contextCompaction",
			payload: `{"type":"contextCompaction","id":"item_8"}`,
			check: func(t *testing.T, item ThreadItem) {
				if item.Item.(*ContextCompactionItem).ID != "item_8" {
					t.Fatal("id")
				}
			},
		},
		{
			name:    "unknown",
			payload: `{"type":"someFutureItem","id":"item_9","extra":{"a":1}}`,
			check: func(t *testing.T, item ThreadItem) {
				unknown, ok := item.Item.(*UnknownItem)
				if !ok {
					t.Fatalf("item = %T", item.Item)
				}
				if unknown.Type != "someFutureItem" || unknown.ID != "item_9" {
					t.Fatalf("unknown = %+v", unknown)
				}
				if len(unknown.Raw) == 0 {
					t.Fatal("raw not preserved")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var item ThreadItem
			if err := json.Unmarshal([]byte(tc.payload), &item); err != nil {
				t.Fatalf("decode: %v", err)
			}
			tc.check(t, item)

			// Round-trip: marshaling re-emits the original JSON.
			out, err := json.Marshal(item)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var a, b any
			if err := json.Unmarshal([]byte(tc.payload), &a); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(out, &b); err != nil {
				t.Fatal(err)
			}
			if string(mustMarshal(t, a)) != string(mustMarshal(t, b)) {
				t.Fatalf("round trip lost data:\n in: %s\nout: %s", tc.payload, out)
			}
		})
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestDecodeItemNotifications(t *testing.T) {
	var started ItemParams
	if err := json.Unmarshal([]byte(`{"threadId":"thr_1","turnId":"turn_1","item":{"type":"enteredReviewMode","id":"turn_900","review":"current changes"}}`), &started); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if started.Item.Type() != ItemEnteredReviewMode || started.Item.ID() != "turn_900" {
		t.Fatalf("item = %+v", started.Item)
	}

	var delta DeltaParams
	if err := json.Unmarshal([]byte(`{"itemId":"item_1","delta":"hel","summaryIndex":2}`), &delta); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if delta.ItemID != "item_1" || delta.Delta != "hel" || delta.SummaryIndex != 2 {
		t.Fatalf("delta = %+v", delta)
	}
}

func TestCommandOutputDeltaText(t *testing.T) {
	plain := CommandOutputDeltaParams{Delta: "hello"}
	if plain.Text() != "hello" {
		t.Fatalf("plain = %q", plain.Text())
	}
	encoded := CommandOutputDeltaParams{DeltaBase64: base64.StdEncoding.EncodeToString([]byte("hi"))}
	if encoded.Text() != "hi" {
		t.Fatalf("encoded = %q", encoded.Text())
	}
	chunk := CommandOutputDeltaParams{Chunk: base64.StdEncoding.EncodeToString([]byte("yo"))}
	if chunk.Text() != "yo" {
		t.Fatalf("chunk = %q", chunk.Text())
	}
	if (CommandOutputDeltaParams{}).Text() != "" {
		t.Fatal("empty delta should decode to empty string")
	}
}

func TestDecodeTurnPlanUpdated(t *testing.T) {
	const payload = `{"turnId":"turn_1","explanation":"why","plan":[{"step":"a","status":"completed"},{"step":"b","status":"inProgress"}]}`
	var params TurnPlanParams
	if err := json.Unmarshal([]byte(payload), &params); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(params.Plan) != 2 || params.Plan[1].Status != PlanStepInProgress {
		t.Fatalf("plan = %+v", params.Plan)
	}
}

func TestEncodeInputItems(t *testing.T) {
	params := StartTurnParams{
		ThreadID: "thr_123",
		Input: []InputItem{
			TextInput{Text: "$skill-creator Add a new skill"},
			SkillInput{Name: "skill-creator", Path: "/Users/me/.codex/skills/skill-creator/SKILL.md"},
			ImageInput{URL: "https://example.com/design.png"},
			LocalImageInput{Path: "/tmp/screenshot.png"},
			MentionInput{Name: "Demo App", Path: "app://demo-app"},
		},
		TurnOptions: TurnOptions{
			Model:          "gpt-5.6-terra",
			ApprovalPolicy: ApprovalUnlessTrusted,
			SandboxPolicy:  SandboxWorkspaceWrite([]string{"/Users/me/project"}, true, nil),
		},
	}

	out, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"threadId":"thr_123","input":[` +
		`{"text":"$skill-creator Add a new skill","type":"text"},` +
		`{"name":"skill-creator","path":"/Users/me/.codex/skills/skill-creator/SKILL.md","type":"skill"},` +
		`{"type":"image","url":"https://example.com/design.png"},` +
		`{"path":"/tmp/screenshot.png","type":"localImage"},` +
		`{"name":"Demo App","path":"app://demo-app","type":"mention"}` +
		`],"model":"gpt-5.6-terra","approvalPolicy":"unlessTrusted",` +
		`"sandboxPolicy":{"type":"workspaceWrite","writableRoots":["/Users/me/project"],"networkAccess":true}}`
	if string(out) != want {
		t.Fatalf("marshal mismatch:\n got: %s\nwant: %s", out, want)
	}
}

func TestEncodeSandboxPolicies(t *testing.T) {
	tests := []struct {
		policy *SandboxPolicy
		want   string
	}{
		{SandboxReadOnly(FullReadAccess()), `{"type":"readOnly","access":{"type":"fullAccess"}}`},
		{SandboxDangerFullAccess(), `{"type":"dangerFullAccess"}`},
		{SandboxExternal(NetworkAccessRestricted), `{"type":"externalSandbox","networkAccess":"restricted"}`},
		{
			SandboxWorkspaceWrite([]string{"/Users/me/project"}, false,
				RestrictedReadAccess(true, "/Users/me/shared-read-only")),
			`{"type":"workspaceWrite","writableRoots":["/Users/me/project"],` +
				`"readOnlyAccess":{"type":"restricted","includePlatformDefaults":true,"readableRoots":["/Users/me/shared-read-only"]},` +
				`"networkAccess":false}`,
		},
	}
	for _, tc := range tests {
		out, err := json.Marshal(tc.policy)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if string(out) != tc.want {
			t.Fatalf("\n got: %s\nwant: %s", out, tc.want)
		}
	}
}

func TestEncodeInitializeParams(t *testing.T) {
	params := InitializeParams{
		ClientInfo: ClientInfo{Name: "my_client", Title: "My Client", Version: "0.1.0"},
		Capabilities: &ClientCapabilities{
			OptOutNotificationMethods: []string{"thread/started", "item/agentMessage/delta"},
		},
	}
	out, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"clientInfo":{"name":"my_client","title":"My Client","version":"0.1.0"},` +
		`"capabilities":{"optOutNotificationMethods":["thread/started","item/agentMessage/delta"]}}`
	if string(out) != want {
		t.Fatalf("\n got: %s\nwant: %s", out, want)
	}
}

func TestDecodeInitializeResult(t *testing.T) {
	var res InitializeResult
	if err := json.Unmarshal([]byte(`{"userAgent":"codex/1.0","platformFamily":"unix","platformOs":"macos"}`), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.UserAgent != "codex/1.0" || res.PlatformFamily != "unix" || res.PlatformOs != "macos" {
		t.Fatalf("res = %+v", res)
	}
}

// TestTurnErrorAccessorsOnANilReceiver keeps a failed turn describable. Turn.Error
// is optional even when Status is TurnFailed, so every caller holding a *Turn can
// reach these methods with nothing behind the pointer.
func TestTurnErrorAccessorsOnANilReceiver(t *testing.T) {
	var failure *TurnError
	if got := failure.Error(); got != "codex: turn failed" {
		t.Errorf("Error() = %q, want %q", got, "codex: turn failed")
	}
	if got := failure.Kind(); got != "" {
		t.Errorf("Kind() = %q, want empty", got)
	}
	if code, ok := failure.HTTPStatusCode(); ok {
		t.Errorf("HTTPStatusCode() = %d, true; want 0, false", code)
	}
}
