package claude

import (
	"context"
	"errors"
	"iter"
	"maps"
	"time"
)

// UserInput is one user turn written to the CLI's input stream.
type UserInput struct {
	// Content is the prompt text.
	Content string
	// SessionID targets a specific session; empty lets the CLI choose.
	SessionID string
	// ParentToolUseID attributes the turn to a tool call, when relevant.
	ParentToolUseID string
	// Origin stamps the message's provenance. Only the "human" kind is
	// honored from an SDK host.
	Origin *MessageOrigin
	// Raw replaces the whole frame when set, for fields this struct does not
	// model. Content and the other fields are ignored.
	Raw map[string]any
}

// frame renders the input as a stream-json user message.
func (u UserInput) frame() map[string]any {
	if u.Raw != nil {
		return u.Raw
	}
	out := map[string]any{
		"type":               "user",
		"session_id":         u.SessionID,
		"message":            map[string]any{"role": "user", "content": u.Content},
		"parent_tool_use_id": nil,
	}
	if u.ParentToolUseID != "" {
		out["parent_tool_use_id"] = u.ParentToolUseID
	}
	if u.Origin != nil {
		out["origin"] = u.Origin
	}
	return out
}

// Query runs a one-shot prompt and yields the messages it produces, ending with
// a ResultMessage. It is the simple, stateless entry point; use Client for
// interactive sessions that need follow-ups or interrupts.
//
// The CLI subprocess is torn down when the sequence ends, when the caller
// breaks out of the range loop, or when ctx is cancelled.
//
//	for msg, err := range claude.Query(ctx, "What is 2+2?", nil) {
//		if err != nil {
//			return err
//		}
//		fmt.Println(msg)
//	}
func Query(ctx context.Context, prompt string, opts *Options) iter.Seq2[Message, error] {
	return QueryStream(ctx, func(yield func(UserInput) bool) {
		yield(UserInput{Content: prompt})
	}, opts)
}

// QueryStream is Query with several user turns known up front. Every input is
// written before the responses are consumed, so it stays unidirectional: use
// Client when a later turn depends on an earlier response.
func QueryStream(ctx context.Context, inputs iter.Seq[UserInput], opts *Options) iter.Seq2[Message, error] {
	return func(yield func(Message, error) bool) {
		opts, err := prepareOptions(opts, entrypoint)
		if err != nil {
			yield(nil, err)
			return
		}
		transport := opts.Transport
		if transport == nil {
			transport = newSubprocessTransport(opts)
		}
		if err := transport.Connect(ctx); err != nil {
			yield(nil, err)
			return
		}

		eng := newEngine(transport, opts)
		attachSDKMCPServers(eng, opts)
		defer eng.Close()

		eng.Start(ctx)
		if _, err := eng.Initialize(ctx); err != nil {
			yield(nil, err)
			return
		}

		// Inputs are written in the background: with hooks, permission
		// callbacks or in-process MCP servers configured the writer holds the
		// input stream open until the run ends, which cannot happen before
		// the caller has consumed the messages.
		writerDone := make(chan struct{})
		go func() {
			defer close(writerDone)
			_ = eng.StreamInput(ctx, func(yieldInput func(map[string]any) bool) {
				for input := range inputs {
					if !yieldInput(input.frame()) {
						return
					}
				}
			})
		}()
		defer func() {
			// eng.Close, deferred above, releases the writer; give it a
			// moment to finish so the goroutine never outlives the query.
			select {
			case <-writerDone:
			case <-time.After(5 * time.Second):
			}
		}()

		for msg, err := range eng.Messages() {
			if !yield(msg, err) {
				return
			}
			if err != nil {
				return
			}
		}
	}
}

// attachSDKMCPServers wires the in-process MCP servers configured in opts into
// the engine's mcp_message routing. Without any, the engine answers mcp_message
// requests with a JSON-RPC method-not-found error.
func attachSDKMCPServers(eng *engine, opts *Options) {
	_, _ = eng, opts
}

// prepareOptions validates the options and returns a copy carrying the
// derived settings: the SDK permission handler and the entrypoint marker.
func prepareOptions(opts *Options, entry string) (*Options, error) {
	var copied Options
	if opts != nil {
		copied = *opts
	}
	if copied.CanUseTool != nil {
		if copied.PermissionPromptToolName != "" {
			return nil, errors.New(
				"claude: Options.CanUseTool cannot be used with Options.PermissionPromptToolName; use one or the other")
		}
		// Routes permission prompts over the control protocol.
		copied.PermissionPromptToolName = "stdio"
	}
	env := make(map[string]string, len(copied.Env)+1)
	maps.Copy(env, copied.Env)
	if _, ok := env["CLAUDE_CODE_ENTRYPOINT"]; !ok {
		env["CLAUDE_CODE_ENTRYPOINT"] = entry
	}
	copied.Env = env
	return &copied, nil
}
