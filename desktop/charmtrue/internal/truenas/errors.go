package truenas

import (
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrClosed               = errors.New("truenas: connection closed")
	ErrAuthenticationFailed = errors.New("truenas: authentication failed")
	ErrOTPRequired          = errors.New("truenas: one-time password is required")
)

// TransportError represents a WebSocket connection, read, or write failure.
type TransportError struct {
	Op  string
	Err error
}

func (e *TransportError) Error() string { return fmt.Sprintf("truenas: %s: %v", e.Op, e.Err) }
func (e *TransportError) Unwrap() error { return e.Err }

// ValidationError represents invalid client-side call input.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("truenas: invalid %s: %s", e.Field, e.Message)
}

// JobError represents a failed or aborted TrueNAS job.
type JobError struct {
	ID        int
	State     string
	Message   string
	Exception string
}

func (e *JobError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("truenas: job %d %s: %s", e.ID, e.State, e.Message)
	}
	return fmt.Sprintf("truenas: job %d %s", e.ID, e.State)
}

// IsOverloaded reports the retryable -32000 concurrent-call limit error.
func IsOverloaded(err error) bool {
	var rpcErr *RPCError
	return errors.As(err, &rpcErr) && rpcErr.Code == -32000
}

// RPCError is an error response returned by TrueNAS.
type RPCError struct {
	Code    int        `json:"code"`
	Message string     `json:"message"`
	Data    *ErrorData `json:"data,omitempty"`
}

// ErrorData contains TrueNAS-specific details attached to a JSON-RPC error.
type ErrorData struct {
	Error       int             `json:"error"`
	ErrName     string          `json:"errname"`
	Reason      string          `json:"reason"`
	Trace       json.RawMessage `json:"trace,omitempty"`
	Extra       json.RawMessage `json:"extra,omitempty"`
	PyException string          `json:"py_exception,omitempty"`
}

func (e *RPCError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Data != nil && e.Data.Reason != "" {
		return fmt.Sprintf("truenas: RPC error %d: %s", e.Code, e.Data.Reason)
	}
	if e.Message != "" {
		return fmt.Sprintf("truenas: RPC error %d: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("truenas: RPC error %d", e.Code)
}
