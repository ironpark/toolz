package claude

import (
	"encoding/json"
	"errors"
	"testing"
)

func mustParse(t *testing.T, line string) Message {
	t.Helper()
	msg, err := ParseMessage([]byte(line))
	if err != nil {
		t.Fatalf("ParseMessage(%s): %v", line, err)
	}
	return msg
}

func TestParseUserMessageStringContent(t *testing.T) {
	t.Parallel()
	msg := mustParse(t, `{"type":"user","message":{"role":"user","content":"Hello"},
	  "uuid":"u1","session_id":"s1","parent_tool_use_id":"tu9"}`)
	um, ok := msg.(*UserMessage)
	if !ok {
		t.Fatalf("got %T, want *UserMessage", msg)
	}
	if um.ContentText == nil || *um.ContentText != "Hello" {
		t.Fatalf("content = %v", um.ContentText)
	}
	if um.Content != nil {
		t.Fatalf("blocks should be nil, got %#v", um.Content)
	}
	if um.UUID != "u1" || um.SessionID != "s1" || um.ParentToolUseID != "tu9" {
		t.Fatalf("unexpected metadata: %+v", um)
	}
}

func TestParseUserMessageBlocks(t *testing.T) {
	t.Parallel()
	msg := mustParse(t, `{"type":"user","message":{"content":[
	  {"type":"text","text":"Let me read this file"},
	  {"type":"tool_use","id":"tu1","name":"Read","input":{"file_path":"/x"}},
	  {"type":"tool_result","tool_use_id":"tu1","content":"file body","is_error":false}
	]}}`)
	um := msg.(*UserMessage)
	if len(um.Content) != 3 {
		t.Fatalf("blocks = %d, want 3", len(um.Content))
	}
	if got := um.Content[0].(*TextBlock).Text; got != "Let me read this file" {
		t.Fatalf("text = %q", got)
	}
	tu := um.Content[1].(*ToolUseBlock)
	if tu.ID != "tu1" || tu.Name != "Read" || tu.Input["file_path"] != "/x" {
		t.Fatalf("tool use = %+v", tu)
	}
	tr := um.Content[2].(*ToolResultBlock)
	if tr.ContentText == nil || *tr.ContentText != "file body" {
		t.Fatalf("tool result content = %v", tr.ContentText)
	}
	if tr.IsError == nil || *tr.IsError {
		t.Fatalf("is_error = %v", tr.IsError)
	}
}

func TestParseUserMessageToolResultListContent(t *testing.T) {
	t.Parallel()
	msg := mustParse(t, `{"type":"user","message":{"content":[
	  {"type":"tool_result","tool_use_id":"tu1","content":[{"type":"text","text":"a"}]}]}}`)
	tr := msg.(*UserMessage).Content[0].(*ToolResultBlock)
	if tr.ContentText != nil {
		t.Fatalf("text content should be nil")
	}
	if len(tr.ContentList) != 1 || tr.ContentList[0]["text"] != "a" {
		t.Fatalf("list content = %#v", tr.ContentList)
	}
}

func TestParseUserMessageOrigin(t *testing.T) {
	t.Parallel()
	for _, content := range []string{`"hi"`, `[{"type":"text","text":"hi"}]`} {
		msg := mustParse(t, `{"type":"user","message":{"content":`+content+`},
		  "origin":{"kind":"peer","from":"agent://a","name":"A","verifiedPeerPid":7,"hop":2}}`)
		o := msg.(*UserMessage).Origin
		if o == nil || o.Kind != OriginPeer || o.From != "agent://a" {
			t.Fatalf("origin = %+v", o)
		}
		if o.VerifiedPeerPID != 7 {
			t.Fatalf("pid = %v", o.VerifiedPeerPID)
		}
		if o.Extra["hop"] != float64(2) {
			t.Fatalf("extra = %v", o.Extra)
		}
	}
	// An origin that carries nothing is treated as absent.
	for _, bad := range []string{`"nope"`, `{}`} {
		msg := mustParse(t, `{"type":"user","message":{"content":"hi"},"origin":`+bad+`}`)
		if o := msg.(*UserMessage).Origin; o != nil {
			t.Fatalf("origin %s should be dropped, got %+v", bad, o)
		}
	}
	// An unreadable kind falls back to unclassified, keeping the raw value and
	// the rest of the origin.
	msg := mustParse(t, `{"type":"user","message":{"content":"hi"},
	  "origin":{"kind":3,"from":"agent://a"}}`)
	o := msg.(*UserMessage).Origin
	if o == nil || o.Kind != OriginUnclassified || o.From != "agent://a" {
		t.Fatalf("origin = %+v", o)
	}
	if o.Extra["kind"] != float64(3) {
		t.Fatalf("extra = %v", o.Extra)
	}
}

func TestParseAssistantMessage(t *testing.T) {
	t.Parallel()
	msg := mustParse(t, `{"type":"assistant","session_id":"s1","uuid":"u1","message":{
	  "id":"msg_1","model":"claude-opus-4-5","stop_reason":"end_turn",
	  "usage":{"input_tokens":5},
	  "content":[
	    {"type":"thinking","thinking":"hmm","signature":"sig"},
	    {"type":"text","text":"Here's my response"},
	    {"type":"tool_use","id":"tu1","name":"Bash","input":{"command":"ls"}},
	    {"type":"server_tool_use","id":"st1","name":"web_search","input":{"query":"go"}},
	    {"type":"advisor_tool_result","tool_use_id":"st1","content":{"type":"advisor_result"}},
	    {"type":"brand_new_block","x":1}
	  ]}}`)
	am, ok := msg.(*AssistantMessage)
	if !ok {
		t.Fatalf("got %T", msg)
	}
	if am.Model != "claude-opus-4-5" || am.MessageID != "msg_1" || am.StopReason != "end_turn" {
		t.Fatalf("unexpected: %+v", am)
	}
	if am.SessionID != "s1" || am.UUID != "u1" || am.Usage["input_tokens"] != float64(5) {
		t.Fatalf("unexpected metadata: %+v", am)
	}
	// The unknown block kind is skipped.
	if len(am.Content) != 5 {
		t.Fatalf("blocks = %d, want 5", len(am.Content))
	}
	th := am.Content[0].(*ThinkingBlock)
	if th.Thinking != "hmm" || th.Signature != "sig" {
		t.Fatalf("thinking = %+v", th)
	}
	stu := am.Content[3].(*ServerToolUseBlock)
	if stu.Name != ServerToolWebSearch {
		t.Fatalf("server tool = %+v", stu)
	}
	str2 := am.Content[4].(*ServerToolResultBlock)
	if str2.ToolUseID != "st1" || str2.Content["type"] != "advisor_result" {
		t.Fatalf("server tool result = %+v", str2)
	}
}

func TestParseSystemMessage(t *testing.T) {
	t.Parallel()
	msg := mustParse(t, `{"type":"system","subtype":"init","model":"opus"}`)
	sm, ok := msg.(*SystemMessage)
	if !ok {
		t.Fatalf("got %T", msg)
	}
	if sm.Subtype != "init" || sm.Data["model"] != "opus" {
		t.Fatalf("unexpected: %+v", sm)
	}
}

func TestParseTaskMessages(t *testing.T) {
	t.Parallel()

	started := mustParse(t, `{"type":"system","subtype":"task_started","task_id":"t1",
	  "description":"do it","uuid":"u1","session_id":"s1","tool_use_id":"tu1","task_type":"bg"}`).(*TaskStartedMessage)
	if started.TaskID != "t1" || started.TaskType != "bg" || started.Subtype != "task_started" {
		t.Fatalf("unexpected: %+v", started)
	}
	if started.Data["description"] != "do it" {
		t.Fatal("raw data should be preserved")
	}

	progress := mustParse(t, `{"type":"system","subtype":"task_progress","task_id":"t1",
	  "description":"working","uuid":"u1","session_id":"s1",
	  "usage":{"total_tokens":10,"tool_uses":2,"duration_ms":30},"last_tool_name":"Bash"}`).(*TaskProgressMessage)
	if progress.Usage != (TaskUsage{TotalTokens: 10, ToolUses: 2, DurationMS: 30}) {
		t.Fatalf("usage = %+v", progress.Usage)
	}

	notif := mustParse(t, `{"type":"system","subtype":"task_notification","task_id":"t1",
	  "status":"completed","output_file":"/tmp/o","summary":"done","uuid":"u1","session_id":"s1"}`).(*TaskNotificationMessage)
	if notif.Status != "completed" || notif.Usage != nil || !TerminalTaskStatuses[notif.Status] {
		t.Fatalf("unexpected: %+v", notif)
	}

	updated := mustParse(t, `{"type":"system","subtype":"task_updated","task_id":"t1",
	  "patch":{"status":"killed","end_time":1}}`).(*TaskUpdatedMessage)
	if updated.Status != "killed" || !TerminalTaskStatuses[updated.Status] {
		t.Fatalf("unexpected: %+v", updated)
	}
	// A patch-less update must not fail.
	bare := mustParse(t, `{"type":"system","subtype":"task_updated"}`).(*TaskUpdatedMessage)
	if bare.Status != "" || bare.Patch == nil {
		t.Fatalf("unexpected: %+v", bare)
	}
}

func TestParseHookEventMessage(t *testing.T) {
	t.Parallel()
	msg := mustParse(t, `{"type":"system","subtype":"hook_response","hook_event":"PreToolUse",
	  "session_id":"s1","uuid":"u1","exit_code":0}`)
	hm, ok := msg.(*HookEventMessage)
	if !ok {
		t.Fatalf("got %T", msg)
	}
	if hm.HookEventName != "PreToolUse" || hm.Subtype != "hook_response" || hm.SessionID != "s1" {
		t.Fatalf("unexpected: %+v", hm)
	}
	// Fallback key names are honored.
	alt := mustParse(t, `{"type":"system","subtype":"hook_started","hook_name":"Stop"}`).(*HookEventMessage)
	if alt.HookEventName != "Stop" {
		t.Fatalf("hook name = %q", alt.HookEventName)
	}
}

func TestParseResultMessage(t *testing.T) {
	t.Parallel()
	msg := mustParse(t, `{"type":"result","subtype":"success","duration_ms":100,
	  "duration_api_ms":90,"is_error":false,"num_turns":2,"session_id":"s1",
	  "total_cost_usd":0.01,"usage":{"input_tokens":3},"result":"done",
	  "structured_output":{"ok":true},
	  "modelUsage":{"claude-opus-4-5":{"inputTokens":3,"outputTokens":4,"costUSD":0.01}},
	  "permission_denials":[{"tool_name":"Bash"}],
	  "deferred_tool_use":{"id":"tu1","name":"Bash","input":{"command":"ls"}},
	  "errors":["bad"," ",1],"api_error_status":529,"uuid":"u1",
	  "terminal_reason":"completed","origin":{"kind":"human"}}`)
	rm, ok := msg.(*ResultMessage)
	if !ok {
		t.Fatalf("got %T", msg)
	}
	if rm.DurationMS != 100 || rm.DurationAPIMS != 90 || rm.NumTurns != 2 || rm.IsError {
		t.Fatalf("unexpected: %+v", rm)
	}
	if rm.TotalCostUSD == nil || *rm.TotalCostUSD != 0.01 {
		t.Fatalf("cost = %v", rm.TotalCostUSD)
	}
	if string(rm.StructuredOutput) != `{"ok":true}` {
		t.Fatalf("structured output = %s", rm.StructuredOutput)
	}
	if mu, ok := rm.ModelUsage["claude-opus-4-5"]; !ok || mu.OutputTokens != 4 {
		t.Fatalf("model usage = %#v", rm.ModelUsage)
	}
	if len(rm.PermissionDenials) != 1 || rm.PermissionDenials[0]["tool_name"] != "Bash" {
		t.Fatalf("denials = %#v", rm.PermissionDenials)
	}
	if rm.DeferredToolUse == nil || rm.DeferredToolUse.Name != "Bash" {
		t.Fatalf("deferred = %+v", rm.DeferredToolUse)
	}
	if len(rm.Errors) != 1 || rm.Errors[0] != "bad" {
		t.Fatalf("errors = %#v", rm.Errors)
	}
	if rm.APIErrorStatus == nil || *rm.APIErrorStatus != 529 {
		t.Fatalf("api status = %v", rm.APIErrorStatus)
	}
	if rm.Origin == nil || rm.Origin.Kind != OriginHuman || rm.TerminalReason != "completed" {
		t.Fatalf("unexpected: %+v", rm)
	}
}

func TestParseStreamEventAndResets(t *testing.T) {
	t.Parallel()
	se := mustParse(t, `{"type":"stream_event","uuid":"u1","session_id":"s1",
	  "event":{"type":"content_block_delta"},"parent_tool_use_id":"tu1"}`).(*StreamEvent)
	if se.Event["type"] != "content_block_delta" || se.ParentToolUseID != "tu1" {
		t.Fatalf("unexpected: %+v", se)
	}

	rl := mustParse(t, `{"type":"rate_limit_event","uuid":"u1","session_id":"s1",
	  "rate_limit_info":{"status":"allowed_warning","resetsAt":1700000000,
	  "rateLimitType":"five_hour","utilization":0.8}}`).(*RateLimitEvent)
	if rl.RateLimitInfo.Status != RateLimitAllowedWarning {
		t.Fatalf("status = %q", rl.RateLimitInfo.Status)
	}
	if rl.RateLimitInfo.ResetsAt == nil || *rl.RateLimitInfo.ResetsAt != 1700000000 {
		t.Fatalf("resets = %v", rl.RateLimitInfo.ResetsAt)
	}
	if rl.RateLimitInfo.Utilization == nil || *rl.RateLimitInfo.Utilization != 0.8 {
		t.Fatalf("utilization = %v", rl.RateLimitInfo.Utilization)
	}
	if rl.RateLimitInfo.Raw["rateLimitType"] != "five_hour" {
		t.Fatal("raw info should be preserved")
	}

	cr := mustParse(t, `{"type":"conversation_reset","new_conversation_id":"c2",
	  "uuid":"u1","session_id":"s1"}`).(*ConversationResetMessage)
	if cr.NewConversationID != "c2" {
		t.Fatalf("unexpected: %+v", cr)
	}
}

func TestParseUnknownTypeIsSkipped(t *testing.T) {
	t.Parallel()
	msg, err := ParseMessage([]byte(`{"type":"brand_new_message","x":1}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != nil {
		t.Fatalf("got %T, want nil", msg)
	}
}

func TestParseMalformedMessages(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		line string
	}{
		{"notJSON", `not json`},
		{"notObject", `[1,2]`},
		{"missingType", `{"foo":"bar"}`},
		{"userMissingMessage", `{"type":"user"}`},
		{"userMissingContent", `{"type":"user","message":{}}`},
		{"userBadBlock", `{"type":"user","message":{"content":["nope"]}}`},
		{"assistantMissingModel", `{"type":"assistant","message":{"content":[]}}`},
		{"assistantContentNotList", `{"type":"assistant","message":{"model":"m","content":"hi"}}`},
		{"assistantBadTextBlock", `{"type":"assistant","message":{"model":"m","content":[{"type":"text"}]}}`},
		{"systemMissingSubtype", `{"type":"system"}`},
		{"taskStartedMissingID", `{"type":"system","subtype":"task_started","description":"d","uuid":"u","session_id":"s"}`},
		{"resultMissingDuration", `{"type":"result","subtype":"success","duration_api_ms":1,"is_error":false,"num_turns":1,"session_id":"s"}`},
		{"resultBadDeferred", `{"type":"result","subtype":"success","duration_ms":1,"duration_api_ms":1,"is_error":false,"num_turns":1,"session_id":"s","deferred_tool_use":{"id":"a","name":"b"}}`},
		{"streamEventMissingEvent", `{"type":"stream_event","uuid":"u","session_id":"s"}`},
		{"rateLimitMissingInfo", `{"type":"rate_limit_event","uuid":"u","session_id":"s"}`},
		{"conversationResetMissingID", `{"type":"conversation_reset","uuid":"u","session_id":"s"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tc.line))
			if err == nil {
				t.Fatalf("expected error, got %#v", msg)
			}
			var pe *MessageParseError
			if !errors.As(err, &pe) {
				t.Fatalf("error = %T (%v), want *MessageParseError", err, err)
			}
			if len(pe.Data) == 0 || !json.Valid(pe.Data) && tc.name != "notJSON" {
				t.Fatalf("error should carry the offending payload: %q", pe.Data)
			}
		})
	}
}
