// Package driver adapts supported agent CLIs to one conversation interface.
//
// The runner resolves configuration, workspaces and MCP files before calling
// New. Implementations in this package only receive the values needed to open
// an agent session, send prompts and return text and token usage.
package driver
