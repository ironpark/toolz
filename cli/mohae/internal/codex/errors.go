package codex

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Sentinel errors returned by the package.
var (
	// ErrClosed is returned when the client or its transport has been closed.
	ErrClosed = errors.New("codex: client closed")
	// ErrNotInitialized is returned when an API call is made before the
	// initialize/initialized handshake completed.
	ErrNotInitialized = errors.New("codex: not initialized")
)

// RPCError is a JSON-RPC 2.0 error object returned by the app-server.
type RPCError struct {
	// Code is the JSON-RPC error code.
	Code int `json:"code"`
	// Message is the human-readable error message.
	Message string `json:"message"`
	// Data carries optional structured error detail.
	Data json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *RPCError) Error() string {
	if len(e.Data) > 0 {
		return fmt.Sprintf("codex: rpc error %d: %s (%s)", e.Code, e.Message, string(e.Data))
	}
	return fmt.Sprintf("codex: rpc error %d: %s", e.Code, e.Message)
}

// JSON-RPC error codes used by the app-server.
const (
	// CodeParseError signals malformed JSON.
	CodeParseError = -32700
	// CodeInvalidRequest signals an invalid request object.
	CodeInvalidRequest = -32600
	// CodeMethodNotFound signals an unknown method.
	CodeMethodNotFound = -32601
	// CodeInvalidParams signals invalid method parameters.
	CodeInvalidParams = -32602
	// CodeInternalError signals an internal server error.
	CodeInternalError = -32603
	// CodeServerOverloaded is returned when request ingress is full and the
	// client should retry with exponential backoff and jitter.
	CodeServerOverloaded = -32001
)

// IsOverloaded reports whether err is an app-server overload error
// (JSON-RPC code -32001) that the caller may retry after a backoff.
func IsOverloaded(err error) bool {
	var rpcErr *RPCError
	if errors.As(err, &rpcErr) {
		return rpcErr.Code == CodeServerOverloaded
	}
	return false
}
