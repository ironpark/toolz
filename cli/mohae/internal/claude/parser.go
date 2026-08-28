package claude

import (
	"encoding/json"
	"fmt"
)

// ParseMessage turns one raw CLI output frame into a typed Message.
//
// It returns (nil, nil) for message types this SDK version does not model, so
// that a newer CLI cannot break an older SDK. A *MessageParseError is returned
// when a recognized message is malformed.
func ParseMessage(data []byte) (Message, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, NewMessageParseError(
			fmt.Sprintf("Invalid message data: %v", err), json.RawMessage(data))
	}
	return parseMessageMap(raw, json.RawMessage(data))
}

// parseMessageMap parses an already-decoded frame. src is the original payload,
// carried on errors for context; it may be nil.
func parseMessageMap(data map[string]any, src json.RawMessage) (Message, error) {
	if data == nil {
		return nil, NewMessageParseError("Invalid message data (expected object)", src)
	}
	if src == nil {
		if b, err := json.Marshal(data); err == nil {
			src = b
		}
	}

	msgType, _ := data["type"].(string)

	// Hook lifecycle events arrive as system messages; route them before the
	// generic system handling below.
	if msgType == "system" {
		if sub, _ := data["subtype"].(string); sub == "hook_started" || sub == "hook_response" {
			name := firstString(data, "hook_event", "hook_name", "hook_event_name")
			return &HookEventMessage{
				SystemMessage: SystemMessage{Subtype: sub, Data: data},
				HookEventName: name,
				SessionID:     str(data["session_id"]),
				UUID:          str(data["uuid"]),
			}, nil
		}
	}

	if msgType == "" {
		return nil, NewMessageParseError("Message missing 'type' field", src)
	}

	switch msgType {
	case "user":
		return parseUserMessage(data, src)
	case "assistant":
		return parseAssistantMessage(data, src)
	case "system":
		return parseSystemMessage(data, src)
	case "result":
		return parseResultMessage(data, src)
	case "stream_event":
		return parseStreamEvent(data, src)
	case "rate_limit_event":
		return parseRateLimitEvent(data, src)
	case "conversation_reset":
		return parseConversationReset(data, src)
	default:
		// Forward-compatible: skip unrecognized message types.
		return nil, nil
	}
}

func parseUserMessage(data map[string]any, src json.RawMessage) (Message, error) {
	inner, ok := data["message"].(map[string]any)
	if !ok {
		return nil, missingField("user", "message", src)
	}
	content, ok := inner["content"]
	if !ok {
		return nil, missingField("user", "content", src)
	}
	msg := &UserMessage{
		UUID:            str(data["uuid"]),
		ParentToolUseID: str(data["parent_tool_use_id"]),
		SessionID:       str(data["session_id"]),
		Origin:          parseOrigin(data),
	}
	if tur, ok := data["tool_use_result"].(map[string]any); ok {
		msg.ToolUseResult = tur
	}
	switch c := content.(type) {
	case string:
		msg.ContentText = &c
	case []any:
		blocks := make([]ContentBlock, 0, len(c))
		for _, item := range c {
			block, ok := item.(map[string]any)
			if !ok {
				return nil, NewMessageParseError(
					"Invalid content block (expected object)", src)
			}
			// The CLI only ever puts these three kinds on a user message.
			switch str(block["type"]) {
			case "text":
				b, err := textBlock(block, src)
				if err != nil {
					return nil, err
				}
				blocks = append(blocks, b)
			case "tool_use":
				b, err := toolUseBlock(block, src)
				if err != nil {
					return nil, err
				}
				blocks = append(blocks, b)
			case "tool_result":
				b, err := toolResultBlock(block, src)
				if err != nil {
					return nil, err
				}
				blocks = append(blocks, b)
			}
		}
		msg.Content = blocks
	default:
		s := fmt.Sprint(content)
		msg.ContentText = &s
	}
	return msg, nil
}

func parseAssistantMessage(data map[string]any, src json.RawMessage) (Message, error) {
	inner, ok := data["message"].(map[string]any)
	if !ok {
		return nil, missingField("assistant", "message", src)
	}
	rawContent, ok := inner["content"]
	if !ok {
		return nil, missingField("assistant", "content", src)
	}
	list, ok := rawContent.([]any)
	if !ok {
		return nil, NewMessageParseError("Invalid assistant content (expected list)", src)
	}
	model, ok := inner["model"].(string)
	if !ok {
		return nil, missingField("assistant", "model", src)
	}
	blocks := make([]ContentBlock, 0, len(list))
	for _, item := range list {
		block, ok := item.(map[string]any)
		if !ok {
			return nil, NewMessageParseError("Invalid content block (expected object)", src)
		}
		var (
			b   ContentBlock
			err error
		)
		switch str(block["type"]) {
		case "text":
			b, err = textBlock(block, src)
		case "thinking":
			b, err = thinkingBlock(block, src)
		case "tool_use":
			b, err = toolUseBlock(block, src)
		case "tool_result":
			b, err = toolResultBlock(block, src)
		case "server_tool_use":
			b, err = serverToolUseBlock(block, src)
		case "advisor_tool_result":
			b, err = serverToolResultBlock(block, src)
		default:
			// Unknown block kinds are skipped for forward compatibility.
			continue
		}
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, b)
	}
	msg := &AssistantMessage{
		Content:         blocks,
		Model:           model,
		ParentToolUseID: str(data["parent_tool_use_id"]),
		Error:           str(data["error"]),
		MessageID:       str(inner["id"]),
		StopReason:      str(inner["stop_reason"]),
		SessionID:       str(data["session_id"]),
		UUID:            str(data["uuid"]),
	}
	if usage, ok := inner["usage"].(map[string]any); ok {
		msg.Usage = usage
	}
	return msg, nil
}

func parseSystemMessage(data map[string]any, src json.RawMessage) (Message, error) {
	subtype, ok := data["subtype"].(string)
	if !ok {
		return nil, missingField("system", "subtype", src)
	}
	base := SystemMessage{Subtype: subtype, Data: data}
	switch subtype {
	case "task_started":
		for _, k := range []string{"task_id", "description", "uuid", "session_id"} {
			if _, ok := data[k].(string); !ok {
				return nil, missingField("system", k, src)
			}
		}
		return &TaskStartedMessage{
			SystemMessage: base,
			TaskID:        str(data["task_id"]),
			Description:   str(data["description"]),
			UUID:          str(data["uuid"]),
			SessionID:     str(data["session_id"]),
			ToolUseID:     str(data["tool_use_id"]),
			TaskType:      str(data["task_type"]),
		}, nil
	case "task_progress":
		for _, k := range []string{"task_id", "description", "uuid", "session_id"} {
			if _, ok := data[k].(string); !ok {
				return nil, missingField("system", k, src)
			}
		}
		usage, ok := data["usage"].(map[string]any)
		if !ok {
			return nil, missingField("system", "usage", src)
		}
		return &TaskProgressMessage{
			SystemMessage: base,
			TaskID:        str(data["task_id"]),
			Description:   str(data["description"]),
			Usage:         taskUsage(usage),
			UUID:          str(data["uuid"]),
			SessionID:     str(data["session_id"]),
			ToolUseID:     str(data["tool_use_id"]),
			LastToolName:  str(data["last_tool_name"]),
		}, nil
	case "task_notification":
		for _, k := range []string{"task_id", "status", "output_file", "summary", "uuid", "session_id"} {
			if _, ok := data[k].(string); !ok {
				return nil, missingField("system", k, src)
			}
		}
		msg := &TaskNotificationMessage{
			SystemMessage: base,
			TaskID:        str(data["task_id"]),
			Status:        str(data["status"]),
			OutputFile:    str(data["output_file"]),
			Summary:       str(data["summary"]),
			UUID:          str(data["uuid"]),
			SessionID:     str(data["session_id"]),
			ToolUseID:     str(data["tool_use_id"]),
		}
		if usage, ok := data["usage"].(map[string]any); ok {
			u := taskUsage(usage)
			msg.Usage = &u
		}
		return msg, nil
	case "task_updated":
		// Parsed defensively: a lifecycle patch may omit any field.
		patch, _ := data["patch"].(map[string]any)
		if patch == nil {
			patch = map[string]any{}
		}
		return &TaskUpdatedMessage{
			SystemMessage: base,
			TaskID:        str(data["task_id"]),
			Patch:         patch,
			Status:        str(patch["status"]),
			SessionID:     str(data["session_id"]),
			UUID:          str(data["uuid"]),
		}, nil
	default:
		return &base, nil
	}
}

func parseResultMessage(data map[string]any, src json.RawMessage) (Message, error) {
	subtype, ok := data["subtype"].(string)
	if !ok {
		return nil, missingField("result", "subtype", src)
	}
	durMS, ok := toInt(data["duration_ms"])
	if !ok {
		return nil, missingField("result", "duration_ms", src)
	}
	durAPIMS, ok := toInt(data["duration_api_ms"])
	if !ok {
		return nil, missingField("result", "duration_api_ms", src)
	}
	isErr, ok := data["is_error"].(bool)
	if !ok {
		return nil, missingField("result", "is_error", src)
	}
	numTurns, ok := toInt(data["num_turns"])
	if !ok {
		return nil, missingField("result", "num_turns", src)
	}
	sessionID, ok := data["session_id"].(string)
	if !ok {
		return nil, missingField("result", "session_id", src)
	}
	msg := &ResultMessage{
		Subtype:        subtype,
		DurationMS:     durMS,
		DurationAPIMS:  durAPIMS,
		IsError:        isErr,
		NumTurns:       numTurns,
		SessionID:      sessionID,
		StopReason:     str(data["stop_reason"]),
		Result:         str(data["result"]),
		UUID:           str(data["uuid"]),
		TerminalReason: str(data["terminal_reason"]),
		Origin:         parseOrigin(data),
		Errors:         normalizeResultErrors(data["errors"]),
		Data:           data,
	}
	if cost, ok := data["total_cost_usd"].(float64); ok {
		msg.TotalCostUSD = &cost
	}
	if usage, ok := data["usage"].(map[string]any); ok {
		msg.Usage = usage
	}
	if so, ok := data["structured_output"]; ok && so != nil {
		if b, err := json.Marshal(so); err == nil {
			msg.StructuredOutput = b
		}
	}
	if mu, ok := data["modelUsage"].(map[string]any); ok {
		msg.ModelUsage = map[string]ModelUsage{}
		for name, v := range mu {
			b, err := json.Marshal(v)
			if err != nil {
				continue
			}
			var one ModelUsage
			if err := json.Unmarshal(b, &one); err == nil {
				msg.ModelUsage[name] = one
			}
		}
	}
	if denials, ok := data["permission_denials"].([]any); ok {
		for _, d := range denials {
			if m, ok := d.(map[string]any); ok {
				msg.PermissionDenials = append(msg.PermissionDenials, m)
			}
		}
	}
	if deferred, ok := data["deferred_tool_use"].(map[string]any); ok {
		for _, k := range []string{"id", "name"} {
			if _, ok := deferred[k].(string); !ok {
				return nil, missingField("result", "deferred_tool_use."+k, src)
			}
		}
		input, ok := deferred["input"].(map[string]any)
		if !ok {
			return nil, missingField("result", "deferred_tool_use.input", src)
		}
		msg.DeferredToolUse = &DeferredToolUse{
			ID:    str(deferred["id"]),
			Name:  str(deferred["name"]),
			Input: input,
		}
	}
	if status, ok := toInt(data["api_error_status"]); ok {
		msg.APIErrorStatus = &status
	}
	return msg, nil
}

func parseStreamEvent(data map[string]any, src json.RawMessage) (Message, error) {
	uuid, ok := data["uuid"].(string)
	if !ok {
		return nil, missingField("stream_event", "uuid", src)
	}
	sessionID, ok := data["session_id"].(string)
	if !ok {
		return nil, missingField("stream_event", "session_id", src)
	}
	event, ok := data["event"].(map[string]any)
	if !ok {
		return nil, missingField("stream_event", "event", src)
	}
	return &StreamEvent{
		UUID:            uuid,
		SessionID:       sessionID,
		Event:           event,
		ParentToolUseID: str(data["parent_tool_use_id"]),
	}, nil
}

func parseRateLimitEvent(data map[string]any, src json.RawMessage) (Message, error) {
	info, ok := data["rate_limit_info"].(map[string]any)
	if !ok {
		return nil, missingField("rate_limit_event", "rate_limit_info", src)
	}
	status, ok := info["status"].(string)
	if !ok {
		return nil, missingField("rate_limit_event", "status", src)
	}
	uuid, ok := data["uuid"].(string)
	if !ok {
		return nil, missingField("rate_limit_event", "uuid", src)
	}
	sessionID, ok := data["session_id"].(string)
	if !ok {
		return nil, missingField("rate_limit_event", "session_id", src)
	}
	rli := RateLimitInfo{
		Status:                status,
		RateLimitType:         str(info["rateLimitType"]),
		OverageStatus:         str(info["overageStatus"]),
		OverageDisabledReason: str(info["overageDisabledReason"]),
		Raw:                   info,
	}
	if n, ok := toInt64(info["resetsAt"]); ok {
		rli.ResetsAt = &n
	}
	if n, ok := toInt64(info["overageResetsAt"]); ok {
		rli.OverageResetsAt = &n
	}
	if f, ok := info["utilization"].(float64); ok {
		rli.Utilization = &f
	}
	return &RateLimitEvent{RateLimitInfo: rli, UUID: uuid, SessionID: sessionID}, nil
}

func parseConversationReset(data map[string]any, src json.RawMessage) (Message, error) {
	for _, k := range []string{"new_conversation_id", "uuid", "session_id"} {
		if _, ok := data[k].(string); !ok {
			return nil, missingField("conversation_reset", k, src)
		}
	}
	return &ConversationResetMessage{
		NewConversationID: str(data["new_conversation_id"]),
		UUID:              str(data["uuid"]),
		SessionID:         str(data["session_id"]),
	}, nil
}

// ---------------------------------------------------------------------------
// Block helpers
// ---------------------------------------------------------------------------

func textBlock(b map[string]any, src json.RawMessage) (ContentBlock, error) {
	text, ok := b["text"].(string)
	if !ok {
		return nil, missingBlockField("text", "text", src)
	}
	return &TextBlock{Text: text}, nil
}

func thinkingBlock(b map[string]any, src json.RawMessage) (ContentBlock, error) {
	thinking, ok := b["thinking"].(string)
	if !ok {
		return nil, missingBlockField("thinking", "thinking", src)
	}
	sig, ok := b["signature"].(string)
	if !ok {
		return nil, missingBlockField("thinking", "signature", src)
	}
	return &ThinkingBlock{Thinking: thinking, Signature: sig}, nil
}

func toolUseBlock(b map[string]any, src json.RawMessage) (ContentBlock, error) {
	id, ok := b["id"].(string)
	if !ok {
		return nil, missingBlockField("tool_use", "id", src)
	}
	name, ok := b["name"].(string)
	if !ok {
		return nil, missingBlockField("tool_use", "name", src)
	}
	input, ok := b["input"].(map[string]any)
	if !ok {
		return nil, missingBlockField("tool_use", "input", src)
	}
	return &ToolUseBlock{ID: id, Name: name, Input: input}, nil
}

func toolResultBlock(b map[string]any, src json.RawMessage) (ContentBlock, error) {
	id, ok := b["tool_use_id"].(string)
	if !ok {
		return nil, missingBlockField("tool_result", "tool_use_id", src)
	}
	out := &ToolResultBlock{ToolUseID: id}
	switch c := b["content"].(type) {
	case string:
		out.ContentText = &c
	case []any:
		for _, item := range c {
			if m, ok := item.(map[string]any); ok {
				out.ContentList = append(out.ContentList, m)
			}
		}
		if out.ContentList == nil {
			out.ContentList = []map[string]any{}
		}
	}
	if isErr, ok := b["is_error"].(bool); ok {
		out.IsError = &isErr
	}
	return out, nil
}

func serverToolUseBlock(b map[string]any, src json.RawMessage) (ContentBlock, error) {
	id, ok := b["id"].(string)
	if !ok {
		return nil, missingBlockField("server_tool_use", "id", src)
	}
	name, ok := b["name"].(string)
	if !ok {
		return nil, missingBlockField("server_tool_use", "name", src)
	}
	input, ok := b["input"].(map[string]any)
	if !ok {
		return nil, missingBlockField("server_tool_use", "input", src)
	}
	return &ServerToolUseBlock{ID: id, Name: name, Input: input}, nil
}

func serverToolResultBlock(b map[string]any, src json.RawMessage) (ContentBlock, error) {
	id, ok := b["tool_use_id"].(string)
	if !ok {
		return nil, missingBlockField("advisor_tool_result", "tool_use_id", src)
	}
	content, ok := b["content"].(map[string]any)
	if !ok {
		return nil, missingBlockField("advisor_tool_result", "content", src)
	}
	return &ServerToolResultBlock{ToolUseID: id, Content: content}, nil
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

// parseOrigin returns data["origin"] when it is a non-empty object, passing
// keys this SDK version does not model through to Extra. A missing or
// non-string kind falls back to OriginUnclassified rather than discarding the
// rest of the origin.
func parseOrigin(data map[string]any) *MessageOrigin {
	raw, ok := data["origin"].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	o := &MessageOrigin{
		Server:       str(raw["server"]),
		From:         str(raw["from"]),
		Name:         str(raw["name"]),
		FromSession:  str(raw["fromSession"]),
		SenderTaskID: str(raw["senderTaskId"]),
		Body:         str(raw["body"]),
		Subkind:      str(raw["subkind"]),
	}
	o.VerifiedPeerPID, _ = toInt(raw["verifiedPeerPid"])
	for k, v := range raw {
		if !originModeledKeys[k] {
			o.putExtra(k, v)
		}
	}
	if kind, ok := raw["kind"].(string); ok && kind != "" {
		o.Kind = kind
	} else {
		o.Kind = OriginUnclassified
		if v := raw["kind"]; v != nil {
			o.putExtra("kind", v)
		}
	}
	return o
}

func taskUsage(m map[string]any) TaskUsage {
	var u TaskUsage
	u.TotalTokens, _ = toInt(m["total_tokens"])
	u.ToolUses, _ = toInt(m["tool_uses"])
	u.DurationMS, _ = toInt(m["duration_ms"])
	return u
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func toInt64(v any) (int64, bool) {
	n, ok := toInt(v)
	return int64(n), ok
}

func missingField(kind, field string, src json.RawMessage) error {
	return NewMessageParseError(
		fmt.Sprintf("Missing required field in %s message: %s", kind, field), src)
}

func missingBlockField(kind, field string, src json.RawMessage) error {
	return NewMessageParseError(
		fmt.Sprintf("Missing required field in %s block: %s", kind, field), src)
}
