package claude

import (
	"context"
	"encoding/json"
	"iter"
)

// Transport moves newline-delimited JSON between this process and a Claude Code
// CLI session. The subprocess implementation is the only one used in
// production; tests substitute fakes.
//
// The lifecycle is: Connect, then any number of Write calls while
// ReadMessages is being ranged over, then EndInput and Close. Close is
// idempotent.
type Transport interface {
	// Connect starts the session. ctx bounds the startup; cancelling it
	// after Connect returns also terminates the session.
	Connect(ctx context.Context) error

	// Write sends one raw frame. Implementations append the trailing
	// newline if the frame lacks one, and are safe for concurrent use.
	Write(ctx context.Context, data []byte) error

	// ReadMessages yields each frame the CLI wrote, in order. The sequence
	// ends after the CLI closes its output; a non-nil error is the final
	// item, and the raw message is nil in that case. ReadMessages must be
	// ranged over at most once.
	ReadMessages() iter.Seq2[json.RawMessage, error]

	// EndInput closes the input side, telling the CLI no more prompts are
	// coming. It is idempotent.
	EndInput() error

	// Close shuts the session down and releases its resources.
	Close() error

	// Ready reports whether the transport can accept writes.
	Ready() bool
}
