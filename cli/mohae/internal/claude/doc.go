// Package claude drives the Claude Code CLI from Go.
//
// It is a port of the official claude-agent-sdk-python: the same wire protocol,
// the same option surface and the same semantics, expressed with contexts,
// iterators and interfaces instead of translated Python idioms.
//
// # Entry points
//
// [Query] runs a one-shot prompt and yields the messages it produces:
//
//	for msg, err := range claude.Query(ctx, "What is 2+2?", nil) {
//		if err != nil {
//			return err
//		}
//		if am, ok := msg.(*claude.AssistantMessage); ok {
//			for _, block := range am.Content {
//				if text, ok := block.(*claude.TextBlock); ok {
//					fmt.Println(text.Text)
//				}
//			}
//		}
//	}
//
// [Client] runs an interactive session where later turns depend on earlier
// responses, and supports interrupts and mid-conversation setters:
//
//	client := claude.NewClient(&claude.Options{PermissionMode: claude.PermissionModeAcceptEdits})
//	if err := client.Connect(ctx); err != nil {
//		return err
//	}
//	defer client.Disconnect()
//	if err := client.Query(ctx, "Summarize this repo", ""); err != nil {
//		return err
//	}
//	for msg, err := range client.ReceiveResponse(ctx) {
//		...
//	}
//
// # Lifecycle
//
// Every blocking call takes a [context.Context]; cancelling it terminates the
// CLI subprocess and unblocks readers. A message sequence ends when the CLI's
// output ends, and a fatal error arrives as the final item of the sequence
// rather than as a panic. Breaking out of a [Query] range loop tears the
// session down; a [Client] is torn down by [Client.Disconnect], which is
// idempotent and safe to defer.
//
// Message and content-block unions are sealed interfaces ([Message],
// [ContentBlock], [PermissionResult], [MCPServerConfig], [ToolContent]): switch
// on the concrete type rather than inspecting maps. Unknown message and block
// kinds from a newer CLI are skipped instead of failing, so an older build of
// this package keeps working.
//
// # Callbacks
//
// [Options.CanUseTool] answers the permission prompts the CLI would otherwise
// show a user, [Options.Hooks] registers lifecycle hooks, and
// [NewSDKMCPServer] exposes in-process tools. All three are served over the
// control protocol: the CLI writes a request and waits for this process to
// answer, so the input stream is held open until the run ends whenever any of
// them is configured.
//
// # Name mapping
//
// The Go names differ from the Python ones where Go conventions differ:
//
//	query()                      -> Query, QueryStream
//	ClaudeSDKClient              -> Client
//	ClaudeAgentOptions           -> Options
//	create_sdk_mcp_server()      -> NewSDKMCPServer
//	tool()                       -> NewTool, ToolDef
//	client.get_mcp_status()      -> Client.MCPServerStatus
//	client.get_context_usage()   -> Client.ContextUsage
//	client.get_server_info()     -> Client.ServerInfo
//	ClaudeSDKError               -> Error
//	CLIJSONDecodeError           -> JSONDecodeError
//	CLIConnectionError           -> ConnectionError
//
// Python's hook output fields async_ and continue_ are [HookOutput.Async] and
// [HookOutput.Continue]; they reach the CLI under their reserved-word names.
//
// # Not ported
//
// The Python SDK's session-store subsystem — external transcript mirroring,
// session listing, session mutation and resume materialization
// (SessionStore, get_session_messages, fork_session and friends) — is out of
// scope. Transcript-mirror frames from the CLI are dropped rather than
// surfaced. Options.User is rejected by the subprocess transport instead of
// being silently ignored: run the process as the desired user instead.
package claude
