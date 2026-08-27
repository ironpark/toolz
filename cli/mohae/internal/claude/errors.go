package claude

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Error is implemented by every error this package originates. It mirrors the
// Python SDK's ClaudeSDKError base class, so callers can write
//
//	var sdkErr claude.Error
//	if errors.As(err, &sdkErr) { ... }
type Error interface {
	error
	claudeSDKError()
}

// baseError carries the message shared by all SDK errors.
type baseError struct {
	Msg string
}

func (e *baseError) Error() string   { return e.Msg }
func (e *baseError) claudeSDKError() {}

// ConnectionError is returned when the SDK cannot talk to the Claude Code CLI.
// Ported from CLIConnectionError.
type ConnectionError struct {
	baseError
}

// NewConnectionError builds a ConnectionError with the given message.
func NewConnectionError(msg string) *ConnectionError {
	return &ConnectionError{baseError{Msg: msg}}
}

// CLINotFoundError is returned when the `claude` executable cannot be located.
// It unwraps to a ConnectionError, mirroring the Python class hierarchy.
type CLINotFoundError struct {
	baseError
	// CLIPath is the path that was searched for, when one was given.
	CLIPath string
}

// NewCLINotFoundError builds a CLINotFoundError. An empty message defaults to
// "Claude Code not found"; a non-empty cliPath is appended to it.
func NewCLINotFoundError(msg, cliPath string) *CLINotFoundError {
	if msg == "" {
		msg = "Claude Code not found"
	}
	if cliPath != "" {
		msg = msg + ": " + cliPath
	}
	return &CLINotFoundError{baseError{Msg: msg}, cliPath}
}

// Unwrap reports a ConnectionError so that errors.As with a *ConnectionError
// target matches, as `except CLIConnectionError` does in Python.
func (e *CLINotFoundError) Unwrap() error { return &ConnectionError{baseError{Msg: e.Msg}} }

// ProcessError is returned when the CLI subprocess fails.
type ProcessError struct {
	baseError
	// ExitCode is the process exit status, or nil when the process did not
	// report one.
	ExitCode *int
	// Stderr holds captured standard error output, possibly truncated.
	Stderr string
}

// NewProcessError builds a ProcessError. exitCode may be nil.
func NewProcessError(msg string, exitCode *int, stderr string) *ProcessError {
	full := msg
	if exitCode != nil {
		full = fmt.Sprintf("%s (exit code: %d)", full, *exitCode)
	}
	if stderr != "" {
		full = full + "\nError output: " + stderr
	}
	return &ProcessError{baseError{Msg: full}, exitCode, stderr}
}

// ResultError is returned when the CLI ends a run by emitting a result message
// with is_error set and then exits non-zero. It unwraps to a ProcessError.
type ResultError struct {
	baseError
	// ExitCode is the process exit status, when reported.
	ExitCode *int
	// Subtype is the result subtype, e.g. "error_max_turns".
	Subtype string
	// Errors holds the error strings reported by the CLI.
	Errors []string
	// Result is the result text, if any.
	Result string
	// APIErrorStatus is the HTTP status of a failing API call, when reported.
	APIErrorStatus *int
	// TerminalReason explains why the run ended, when reported.
	TerminalReason string
	// SessionID is the session the result belongs to, when reported.
	SessionID string
	// Data is the raw result payload as emitted by the CLI.
	Data map[string]any
}

// NewResultError builds a ResultError from a raw result message payload.
func NewResultError(msg string, data map[string]any, exitCode *int) *ResultError {
	full := msg
	if exitCode != nil {
		full = fmt.Sprintf("%s (exit code: %d)", full, *exitCode)
	}
	e := &ResultError{baseError: baseError{Msg: full}, ExitCode: exitCode, Data: data}
	if data == nil {
		return e
	}
	if s, ok := data["subtype"].(string); ok {
		e.Subtype = s
	}
	e.Errors = normalizeResultErrors(data["errors"])
	if s, ok := data["result"].(string); ok {
		e.Result = s
	}
	if n, ok := toInt(data["api_error_status"]); ok {
		e.APIErrorStatus = &n
	}
	if s, ok := data["terminal_reason"].(string); ok {
		e.TerminalReason = s
	}
	if s, ok := data["session_id"].(string); ok {
		e.SessionID = s
	}
	return e
}

// Unwrap reports a ProcessError so that `errors.As` with a *ProcessError target
// matches, as `except ProcessError` does in Python.
func (e *ResultError) Unwrap() error {
	return &ProcessError{baseError{Msg: e.Msg}, e.ExitCode, ""}
}

// normalizeResultErrors cleans the `errors` field of a result frame: a bare
// string is promoted to a single-element list, non-strings and blanks dropped.
func normalizeResultErrors(raw any) []string {
	var items []any
	switch v := raw.(type) {
	case string:
		items = []any{v}
	case []any:
		items = v
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
	var out []string
	for _, it := range items {
		s, ok := it.(string)
		if !ok {
			continue
		}
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	}
	return 0, false
}

// JSONDecodeError is returned when a line of CLI output is not valid JSON.
// Ported from CLIJSONDecodeError.
type JSONDecodeError struct {
	baseError
	// Line is the offending output line.
	Line string
	// Err is the underlying decoding error.
	Err error
}

// NewJSONDecodeError builds a JSONDecodeError for the given line.
func NewJSONDecodeError(line string, err error) *JSONDecodeError {
	trunc := line
	if len(trunc) > 100 {
		trunc = trunc[:100]
	}
	return &JSONDecodeError{
		baseError: baseError{Msg: "Failed to decode JSON: " + trunc + "..."},
		Line:      line,
		Err:       err,
	}
}

// Unwrap returns the underlying decoding error.
func (e *JSONDecodeError) Unwrap() error { return e.Err }

// MessageParseError is returned when a well-formed JSON payload cannot be
// turned into a typed Message.
type MessageParseError struct {
	baseError
	// Data is the offending payload, when available.
	Data json.RawMessage
}

// NewMessageParseError builds a MessageParseError. data may be nil.
func NewMessageParseError(msg string, data json.RawMessage) *MessageParseError {
	return &MessageParseError{baseError{Msg: msg}, data}
}
