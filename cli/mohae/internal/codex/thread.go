package codex

import (
	"context"
	"encoding/json"
	"iter"
	"sync"
)

// ThreadEvent is a thread lifecycle notification delivered to a subscriber.
type ThreadEvent struct {
	// Method is the notification method, one of the MethodThread* constants.
	Method string
	// ThreadID is the thread the event belongs to.
	ThreadID string
	// Thread is set for thread/started.
	Thread *Thread
	// Status is set for thread/status/changed.
	Status *ThreadStatus
	// Name is set for thread/name/updated.
	Name string
	// Params is the raw notification payload.
	Params json.RawMessage
}

// threadSubscription tracks one subscribed thread and its active turns.
type threadSubscription struct {
	id     string
	events chan ThreadEvent
	queue  chan queuedNotification
	quit   chan struct{}

	mu      sync.Mutex
	closed  bool
	streams []*TurnStream
}

// emit delivers an event without ever blocking the transport reader. A slow
// consumer loses events rather than stalling the connection.
func (s *threadSubscription) emit(c *Client, event ThreadEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.events <- event:
	default:
		c.logger.Debug("codex: dropped thread event", "threadId", s.id, "method", event.Method)
	}
}

// close stops delivery and closes the event channel.
func (s *threadSubscription) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.events)
	close(s.quit)
}

// eventBuffer returns the configured per-subscription channel capacity.
func (c *Client) eventBuffer() int {
	if c.opts.EventBuffer > 0 {
		return c.opts.EventBuffer
	}
	return defaultEventBuffer
}

// subscribe registers interest in a thread's notifications. The app-server
// subscribes the connection automatically on thread/start, thread/resume, and
// thread/fork; this mirrors that state on the client side.
func (c *Client) subscribe(threadID string) *threadSubscription {
	if threadID == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if sub, ok := c.threads[threadID]; ok {
		return sub
	}
	sub := &threadSubscription{
		id:     threadID,
		events: make(chan ThreadEvent, c.eventBuffer()),
		queue:  make(chan queuedNotification, 4*c.eventBuffer()),
		quit:   make(chan struct{}),
	}
	c.threads[threadID] = sub
	go sub.pump(c)
	return sub
}

// lookup returns the subscription for a thread, if any.
func (c *Client) lookup(threadID string) *threadSubscription {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.threads[threadID]
}

// unsubscribeLocal removes a thread subscription and closes its channel.
func (c *Client) unsubscribeLocal(threadID string) {
	c.mu.Lock()
	sub := c.threads[threadID]
	delete(c.threads, threadID)
	c.mu.Unlock()
	if sub != nil {
		sub.close()
	}
}

// ThreadEvents returns the lifecycle event channel for a subscribed thread, or
// nil when the thread is not subscribed. The channel is closed by
// UnsubscribeThread and when the client shuts down. Events are dropped rather
// than queued without bound, so read them promptly.
func (c *Client) ThreadEvents(threadID string) <-chan ThreadEvent {
	if sub := c.lookup(threadID); sub != nil {
		return sub.events
	}
	return nil
}

// routeThreadNotification delivers a thread lifecycle notification to its
// subscriber.
func (c *Client) routeThreadNotification(method string, params json.RawMessage, threadID string) {
	switch method {
	case MethodThreadStarted, MethodThreadStatusChanged, MethodThreadArchived,
		MethodThreadUnarchived, MethodThreadDeleted, MethodThreadClosed,
		MethodThreadNameUpdated:
	default:
		return
	}
	sub := c.lookup(threadID)
	if sub == nil {
		c.logger.Debug("codex: event for unknown thread", "threadId", threadID, "method", method)
		return
	}

	event := ThreadEvent{Method: method, ThreadID: threadID, Params: params}
	switch method {
	case MethodThreadStarted:
		var payload ThreadStartedParams
		if err := json.Unmarshal(params, &payload); err == nil {
			event.Thread = &payload.Thread
		}
	case MethodThreadStatusChanged:
		var payload ThreadStatusChangedParams
		if err := json.Unmarshal(params, &payload); err == nil {
			event.Status = &payload.Status
		}
	case MethodThreadNameUpdated:
		var payload ThreadNameUpdatedParams
		if err := json.Unmarshal(params, &payload); err == nil {
			event.Name = payload.Name
		}
	}
	sub.emit(c, event)
}

// StartThread creates a new thread and subscribes to its turn and item events.
func (c *Client) StartThread(ctx context.Context, params StartThreadParams) (*Thread, error) {
	var result ThreadResult
	if err := c.call(ctx, "thread/start", params, &result); err != nil {
		return nil, err
	}
	c.subscribe(result.Thread.ID)
	return &result.Thread, nil
}

// ResumeThread reopens a stored thread so later turns append to it.
func (c *Client) ResumeThread(ctx context.Context, params ResumeThreadParams) (*Thread, error) {
	var result ThreadResult
	if err := c.call(ctx, "thread/resume", params, &result); err != nil {
		return nil, err
	}
	c.subscribe(result.Thread.ID)
	return &result.Thread, nil
}

// ForkThread branches a stored thread into a new thread id.
func (c *Client) ForkThread(ctx context.Context, params ForkThreadParams) (*Thread, error) {
	var result ThreadResult
	if err := c.call(ctx, "thread/fork", params, &result); err != nil {
		return nil, err
	}
	c.subscribe(result.Thread.ID)
	return &result.Thread, nil
}

// ReadThread reads a stored thread without resuming or subscribing to it.
func (c *Client) ReadThread(ctx context.Context, threadID string, includeTurns bool) (*Thread, error) {
	var result ThreadResult
	params := ReadThreadParams{ThreadID: threadID, IncludeTurns: includeTurns}
	if err := c.call(ctx, "thread/read", params, &result); err != nil {
		return nil, err
	}
	return &result.Thread, nil
}

// ListThreads returns one page of stored threads. An empty NextCursor means
// the final page.
func (c *Client) ListThreads(ctx context.Context, params ListThreadsParams) (*ListThreadsResult, error) {
	var result ListThreadsResult
	if err := c.call(ctx, "thread/list", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// AllThreads iterates every stored thread matching params, following
// nextCursor to exhaustion. Iteration stops after yielding a non-nil error.
func (c *Client) AllThreads(ctx context.Context, params ListThreadsParams) iter.Seq2[Thread, error] {
	return func(yield func(Thread, error) bool) {
		page := params
		for {
			result, err := c.ListThreads(ctx, page)
			if err != nil {
				yield(Thread{}, err)
				return
			}
			for _, thread := range result.Data {
				if !yield(thread, nil) {
					return
				}
			}
			if result.NextCursor == "" {
				return
			}
			page.Cursor = result.NextCursor
		}
	}
}

// ArchiveThread moves a thread's log into the archived directory, along with
// spawned descendant threads.
func (c *Client) ArchiveThread(ctx context.Context, threadID string) error {
	return c.call(ctx, "thread/archive", ThreadIDParams{ThreadID: threadID}, nil)
}

// UnarchiveThread restores an archived thread and returns it.
func (c *Client) UnarchiveThread(ctx context.Context, threadID string) (*Thread, error) {
	var result ThreadResult
	if err := c.call(ctx, "thread/unarchive", ThreadIDParams{ThreadID: threadID}, &result); err != nil {
		return nil, err
	}
	return &result.Thread, nil
}

// DeleteThread permanently deletes a stored thread and its spawned
// descendants.
func (c *Client) DeleteThread(ctx context.Context, threadID string) error {
	return c.call(ctx, "thread/delete", ThreadIDParams{ThreadID: threadID}, nil)
}

// UnsubscribeThread drops this connection's subscription to a thread and
// closes its event channel. The returned status is "unsubscribed",
// "notSubscribed", or "notLoaded".
func (c *Client) UnsubscribeThread(ctx context.Context, threadID string) (string, error) {
	var result UnsubscribeResult
	err := c.call(ctx, "thread/unsubscribe", ThreadIDParams{ThreadID: threadID}, &result)
	c.unsubscribeLocal(threadID)
	if err != nil {
		return "", err
	}
	return result.Status, nil
}

// SetThreadName sets a thread's user-facing name.
func (c *Client) SetThreadName(ctx context.Context, threadID, name string) error {
	params := struct {
		ThreadID string `json:"threadId"`
		Name     string `json:"name"`
	}{ThreadID: threadID, Name: name}
	return c.call(ctx, "thread/name/set", params, nil)
}

// ListLoadedThreads returns the thread ids currently loaded in memory.
func (c *Client) ListLoadedThreads(ctx context.Context) ([]string, error) {
	var result struct {
		Data []string `json:"data"`
	}
	if err := c.call(ctx, "thread/loaded/list", nil, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// shutdown closes every subscription once the transport stops.
func (c *Client) shutdown() {
	c.mu.Lock()
	subs := c.threads
	c.threads = make(map[string]*threadSubscription)
	c.mu.Unlock()

	for _, sub := range subs {
		for _, stream := range sub.activeStreams() {
			stream.finish(nil, ErrClosed)
		}
		sub.close()
	}
}
