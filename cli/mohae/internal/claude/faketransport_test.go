package claude

import (
	"context"
	"encoding/json"
	"iter"
	"sync"
	"testing"
	"time"
)

// fakeTransport is an in-memory Transport standing in for the CLI subprocess.
// Tests push frames with push and observe what the SDK wrote with writes or
// nextWrite.
type fakeTransport struct {
	mu       sync.Mutex
	writes   [][]byte
	ready    bool
	ended    bool
	closed   bool
	readErr  error
	onWrite  func(frame map[string]any)
	in       chan json.RawMessage
	closedCh chan struct{}
	writeCh  chan []byte
	inOnce   sync.Once
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{
		in:       make(chan json.RawMessage, 64),
		closedCh: make(chan struct{}),
		writeCh:  make(chan []byte, 64),
	}
}

func (f *fakeTransport) Connect(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ready = true
	return nil
}

func (f *fakeTransport) Write(_ context.Context, data []byte) error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return NewConnectionError("transport is closed")
	}
	cp := append([]byte(nil), data...)
	f.writes = append(f.writes, cp)
	onWrite := f.onWrite
	f.mu.Unlock()

	select {
	case f.writeCh <- cp:
	default:
	}
	if onWrite != nil {
		var frame map[string]any
		if json.Unmarshal(cp, &frame) == nil {
			onWrite(frame)
		}
	}
	return nil
}

func (f *fakeTransport) ReadMessages() iter.Seq2[json.RawMessage, error] {
	return func(yield func(json.RawMessage, error) bool) {
		for {
			select {
			case raw, ok := <-f.in:
				if !ok {
					f.mu.Lock()
					err := f.readErr
					f.mu.Unlock()
					if err != nil {
						yield(nil, err)
					}
					return
				}
				if !yield(raw, nil) {
					return
				}
			case <-f.closedCh:
				return
			}
		}
	}
}

func (f *fakeTransport) EndInput() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ended = true
	f.ready = false
	return nil
}

func (f *fakeTransport) Close() error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	f.ready = false
	f.mu.Unlock()
	close(f.closedCh)
	return nil
}

func (f *fakeTransport) Ready() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ready
}

// push queues one frame for the SDK to read.
func (f *fakeTransport) push(v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	select {
	case f.in <- raw:
	case <-f.closedCh:
	}
}

// finish ends the CLI's output, optionally with a fatal error.
func (f *fakeTransport) finish(err error) {
	f.mu.Lock()
	f.readErr = err
	f.mu.Unlock()
	f.inOnce.Do(func() { close(f.in) })
}

// endedInput reports whether EndInput was called.
func (f *fakeTransport) endedInput() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ended
}

// frames returns everything the SDK wrote, decoded.
func (f *fakeTransport) frames(t *testing.T) []map[string]any {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]map[string]any, 0, len(f.writes))
	for _, w := range f.writes {
		var frame map[string]any
		if err := json.Unmarshal(w, &frame); err != nil {
			t.Fatalf("write %s is not JSON: %v", w, err)
		}
		out = append(out, frame)
	}
	return out
}

// nextWrite waits for the next frame the SDK writes.
func (f *fakeTransport) nextWrite(t *testing.T) map[string]any {
	t.Helper()
	select {
	case raw := <-f.writeCh:
		var frame map[string]any
		if err := json.Unmarshal(raw, &frame); err != nil {
			t.Fatalf("write %s is not JSON: %v", raw, err)
		}
		return frame
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a write")
		return nil
	}
}

// nextResponse waits for the next control_response the SDK writes, skipping
// the requests it sends of its own accord.
func (f *fakeTransport) nextResponse(t *testing.T) map[string]any {
	t.Helper()
	for {
		frame := f.nextWrite(t)
		if frame["type"] == "control_response" {
			return frame["response"].(map[string]any)
		}
	}
}

// respondSuccess makes the fake answer every control request the SDK sends with
// the given payload.
func (f *fakeTransport) respondSuccess(payload map[string]any) {
	f.mu.Lock()
	f.onWrite = func(frame map[string]any) {
		if frame["type"] != "control_request" {
			return
		}
		f.push(map[string]any{
			"type": "control_response",
			"response": map[string]any{
				"subtype":    "success",
				"request_id": frame["request_id"],
				"response":   payload,
			},
		})
	}
	f.mu.Unlock()
}
