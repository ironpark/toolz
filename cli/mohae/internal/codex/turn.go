package codex

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
)

// ErrTurnAbandoned is reported by Wait when the caller closed the stream
// before the turn reached a terminal status.
var ErrTurnAbandoned = errors.New("codex: turn stream abandoned")

// EventKind classifies a turn event.
type EventKind string

// Turn event kinds delivered on a TurnStream.
const (
	EventTurnStarted        EventKind = "turnStarted"
	EventTurnCompleted      EventKind = "turnCompleted"
	EventItemStarted        EventKind = "itemStarted"
	EventItemCompleted      EventKind = "itemCompleted"
	EventAgentMessageDelta  EventKind = "agentMessageDelta"
	EventPlanDelta          EventKind = "planDelta"
	EventReasoningDelta     EventKind = "reasoningDelta"
	EventCommandOutputDelta EventKind = "commandOutputDelta"
	EventPlanUpdated        EventKind = "planUpdated"
	EventDiffUpdated        EventKind = "diffUpdated"
	EventTokenUsageUpdated  EventKind = "tokenUsageUpdated"
)

// Event is one streamed update for a turn.
type Event struct {
	// Kind classifies the event.
	Kind EventKind
	// Method is the originating notification method.
	Method string
	// ThreadID and TurnID identify the turn the event belongs to.
	ThreadID string
	TurnID   string

	// Turn is set for EventTurnStarted and EventTurnCompleted.
	Turn *Turn
	// Item is set for EventItemStarted and EventItemCompleted.
	Item *ThreadItem
	// ItemID identifies the item a delta appends to.
	ItemID string
	// Delta is the appended text for the delta events, or the decoded output
	// chunk for EventCommandOutputDelta.
	Delta string
	// Stream is "stdout" or "stderr" for EventCommandOutputDelta.
	Stream string
	// SummaryIndex increments when a new reasoning summary section opens.
	SummaryIndex int
	// Reasoning reports whether a reasoning delta is a summary or raw text.
	ReasoningSummary bool
	// Plan and Explanation are set for EventPlanUpdated.
	Plan        []PlanStep
	Explanation string
	// Diff is the aggregated unified diff for EventDiffUpdated.
	Diff string
	// Usage is set for EventTokenUsageUpdated.
	Usage *TokenUsage
	// Params is the raw notification payload.
	Params json.RawMessage
}

// TurnStream delivers a turn's events in arrival order. The channel returned
// by Events is closed once the turn reaches a terminal status, the caller
// calls Close, or the client shuts down.
type TurnStream struct {
	client   *Client
	threadID string
	events   chan Event
	done     chan struct{}

	mu       sync.Mutex
	turnID   string
	finished bool
	final    *Turn
	err      error

	doneOnce  sync.Once
	closeOnce sync.Once
}

// ThreadID returns the thread the turn belongs to.
func (s *TurnStream) ThreadID() string { return s.threadID }

// TurnID returns the turn id, which is empty until turn/start returns.
func (s *TurnStream) TurnID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.turnID
}

// Events returns the stream's event channel.
func (s *TurnStream) Events() <-chan Event { return s.events }

// Done returns a channel closed when the turn ends for any reason.
func (s *TurnStream) Done() <-chan struct{} { return s.done }

// Wait blocks until the turn reaches a terminal status and returns the final
// turn. It returns an error when the context ends first, when the client shut
// down, or when the stream was abandoned.
func (s *TurnStream) Wait(ctx context.Context) (*Turn, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.err != nil {
			return s.final, s.err
		}
		return s.final, nil
	}
}

// Close abandons the stream. Pending and future events for this turn are
// discarded; the turn itself keeps running on the server until it completes or
// is interrupted.
func (s *TurnStream) Close() {
	s.finish(nil, ErrTurnAbandoned)
}

// finish records the terminal state and releases everyone waiting. The event
// channel is closed by the owning thread pump, which is the only goroutine
// that sends on it.
func (s *TurnStream) finish(final *Turn, err error) {
	s.mu.Lock()
	if !s.finished {
		s.finished = true
		s.final = final
		s.err = err
	}
	s.mu.Unlock()
	s.doneOnce.Do(func() { close(s.done) })
}

// closeEvents closes the event channel exactly once.
func (s *TurnStream) closeEvents() {
	s.closeOnce.Do(func() { close(s.events) })
}

// deliver sends an event, waiting for the consumer. It returns false when the
// stream was abandoned or the client shut down, in which case the event and
// every later one is dropped.
func (s *TurnStream) deliver(event Event, quit <-chan struct{}) bool {
	select {
	case s.events <- event:
		return true
	case <-s.done:
		return false
	case <-quit:
		return false
	}
}

// queuedNotification is one turn notification waiting for its subscriber.
type queuedNotification struct {
	method   string
	params   json.RawMessage
	threadID string
	turnID   string
}

// newStream registers a pending turn stream on a thread subscription.
func (s *threadSubscription) newStream(c *Client, threadID string) *TurnStream {
	stream := &TurnStream{
		client:   c,
		threadID: threadID,
		events:   make(chan Event, c.eventBuffer()),
		done:     make(chan struct{}),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streams = append(s.streams, stream)
	return stream
}

// bindTurnID assigns the id returned by turn/start to a pending stream.
func (s *threadSubscription) bindTurnID(stream *TurnStream, turnID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stream.mu.Lock()
	if stream.turnID == "" {
		stream.turnID = turnID
	}
	stream.mu.Unlock()
}

// removeStream drops a stream from the subscription.
func (s *threadSubscription) removeStream(target *TurnStream) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, stream := range s.streams {
		if stream == target {
			s.streams = append(s.streams[:index], s.streams[index+1:]...)
			return
		}
	}
}

// streamFor resolves the stream a notification belongs to. A pending stream
// adopts the first turn id it sees, which covers events that race ahead of the
// turn/start response.
func (s *threadSubscription) streamFor(turnID string) *TurnStream {
	s.mu.Lock()
	defer s.mu.Unlock()
	if turnID != "" {
		for _, stream := range s.streams {
			if stream.TurnID() == turnID {
				return stream
			}
		}
	}
	for _, stream := range s.streams {
		if stream.TurnID() == "" {
			if turnID != "" {
				stream.mu.Lock()
				stream.turnID = turnID
				stream.mu.Unlock()
			}
			return stream
		}
	}
	if len(s.streams) == 1 {
		return s.streams[0]
	}
	return nil
}

// activeStreams returns a snapshot of the subscription's streams.
func (s *threadSubscription) activeStreams() []*TurnStream {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*TurnStream(nil), s.streams...)
}

// enqueue hands a notification to the thread's pump. It never blocks the
// transport reader: when the queue is full the notification is dropped.
func (s *threadSubscription) enqueue(c *Client, note queuedNotification) {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return
	}
	select {
	case s.queue <- note:
	default:
		c.logger.Debug("codex: dropped turn notification", "threadId", s.id, "method", note.method)
	}
}

// pump fans notifications out to turn streams. One pump runs per subscribed
// thread, so a slow consumer stalls only its own thread.
func (s *threadSubscription) pump(c *Client) {
	defer func() {
		for _, stream := range s.activeStreams() {
			stream.finish(nil, ErrClosed)
			stream.closeEvents()
		}
	}()
	for {
		select {
		case <-s.quit:
			return
		case note := <-s.queue:
			c.deliverTurnEvent(s, note)
		}
	}
}

// deliverTurnEvent routes one queued notification to its stream.
func (c *Client) deliverTurnEvent(sub *threadSubscription, note queuedNotification) {
	stream := sub.streamFor(note.turnID)
	if stream == nil {
		c.logger.Debug("codex: event for unknown turn", "threadId", note.threadID,
			"turnId", note.turnID, "method", note.method)
		return
	}
	event, ok := buildEvent(note)
	if !ok {
		return
	}
	if event.ThreadID == "" {
		event.ThreadID = sub.id
	}
	if event.TurnID == "" {
		event.TurnID = stream.TurnID()
	}

	if !stream.deliver(event, sub.quit) {
		// The consumer abandoned the stream or the client is shutting down.
		sub.removeStream(stream)
		stream.closeEvents()
		return
	}
	if event.Kind == EventTurnStarted {
		// A new turn clears prompts left over from the previous one.
		c.pending.cancelTurn(turnKey(event.ThreadID, event.TurnID))
	}
	if event.Kind == EventTurnCompleted {
		// Pending approval prompts for this turn can no longer be answered.
		c.pending.cancelTurn(turnKey(event.ThreadID, event.TurnID))
		stream.finish(event.Turn, nil)
		sub.removeStream(stream)
		stream.closeEvents()
	}
}

// turnMethods is the set of notifications routed to turn streams.
func isTurnMethod(method string) bool {
	switch method {
	case MethodTurnStarted, MethodTurnCompleted, MethodTurnDiff, MethodTurnPlan,
		MethodItemStarted, MethodItemCompleted, MethodAgentMessageDelta,
		MethodPlanDelta, MethodReasoningSummaryTextDelta,
		MethodReasoningSummaryPartAdded, MethodReasoningTextDelta,
		MethodCommandExecutionOutputDlta, MethodTokenUsageUpdated:
		return true
	default:
		return false
	}
}

// buildEvent decodes a notification into a typed event.
func buildEvent(note queuedNotification) (Event, bool) {
	event := Event{
		Method:   note.method,
		ThreadID: note.threadID,
		TurnID:   note.turnID,
		Params:   note.params,
	}
	switch note.method {
	case MethodTurnStarted, MethodTurnCompleted:
		var payload TurnParams
		if err := json.Unmarshal(note.params, &payload); err != nil {
			return event, false
		}
		event.Turn = &payload.Turn
		if event.TurnID == "" {
			event.TurnID = payload.Turn.ID
		}
		if note.method == MethodTurnStarted {
			event.Kind = EventTurnStarted
		} else {
			event.Kind = EventTurnCompleted
		}
	case MethodItemStarted, MethodItemCompleted:
		var payload ItemParams
		if err := json.Unmarshal(note.params, &payload); err != nil {
			return event, false
		}
		item := payload.Item
		event.Item = &item
		event.ItemID = item.ID()
		if note.method == MethodItemStarted {
			event.Kind = EventItemStarted
		} else {
			event.Kind = EventItemCompleted
		}
	case MethodAgentMessageDelta, MethodPlanDelta, MethodReasoningTextDelta,
		MethodReasoningSummaryTextDelta, MethodReasoningSummaryPartAdded:
		var payload DeltaParams
		if err := json.Unmarshal(note.params, &payload); err != nil {
			return event, false
		}
		event.ItemID = payload.ItemID
		event.Delta = payload.Delta
		event.SummaryIndex = payload.SummaryIndex
		switch note.method {
		case MethodAgentMessageDelta:
			event.Kind = EventAgentMessageDelta
		case MethodPlanDelta:
			event.Kind = EventPlanDelta
		default:
			event.Kind = EventReasoningDelta
			event.ReasoningSummary = note.method != MethodReasoningTextDelta
		}
	case MethodCommandExecutionOutputDlta:
		var payload CommandOutputDeltaParams
		if err := json.Unmarshal(note.params, &payload); err != nil {
			return event, false
		}
		event.Kind = EventCommandOutputDelta
		event.ItemID = payload.ItemID
		event.Stream = payload.Stream
		event.Delta = payload.Text()
	case MethodTurnPlan:
		var payload TurnPlanParams
		if err := json.Unmarshal(note.params, &payload); err != nil {
			return event, false
		}
		event.Kind = EventPlanUpdated
		event.Plan = payload.Plan
		event.Explanation = payload.Explanation
	case MethodTurnDiff:
		var payload TurnDiffParams
		if err := json.Unmarshal(note.params, &payload); err != nil {
			return event, false
		}
		event.Kind = EventDiffUpdated
		event.Diff = payload.Diff
	case MethodTokenUsageUpdated:
		var payload TokenUsageParams
		if err := json.Unmarshal(note.params, &payload); err != nil {
			return event, false
		}
		event.Kind = EventTokenUsageUpdated
		usage := payload.Usage
		event.Usage = &usage
	default:
		return event, false
	}
	return event, true
}

// routeTurnNotification queues a turn or item notification for its thread
// pump. It reports whether the notification was a turn-scoped one.
func (c *Client) routeTurnNotification(method string, params json.RawMessage, threadID, turnID string) bool {
	if !isTurnMethod(method) {
		return false
	}
	sub := c.lookup(threadID)
	if sub == nil && threadID == "" {
		// Some notifications omit threadId. With a single subscribed thread
		// the target is unambiguous; otherwise the event is dropped.
		sub = c.soleSubscription()
	}
	if sub == nil {
		c.logger.Debug("codex: event for unknown thread", "threadId", threadID, "method", method)
		return true
	}
	sub.enqueue(c, queuedNotification{method: method, params: params, threadID: threadID, turnID: turnID})
	return true
}

// soleSubscription returns the only subscribed thread, if there is exactly
// one.
func (c *Client) soleSubscription() *threadSubscription {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.threads) != 1 {
		return nil
	}
	for _, sub := range c.threads {
		return sub
	}
	return nil
}

// StartTurn adds user input to a thread, begins Codex generation, and returns
// a stream of the turn's events. Close the stream or drain it to completion.
func (c *Client) StartTurn(ctx context.Context, threadID string, input []InputItem, opts *TurnOptions) (*TurnStream, error) {
	sub := c.subscribe(threadID)
	if sub == nil {
		return nil, errors.New("codex: StartTurn requires a thread id")
	}
	stream := sub.newStream(c, threadID)

	params := StartTurnParams{ThreadID: threadID, Input: input}
	if opts != nil {
		params.TurnOptions = *opts
	}
	var result StartTurnResult
	if err := c.call(ctx, "turn/start", params, &result); err != nil {
		sub.removeStream(stream)
		stream.finish(nil, err)
		stream.closeEvents()
		return nil, err
	}
	sub.bindTurnID(stream, result.Turn.ID)
	return stream, nil
}

// SteerTurn appends user input to the active in-flight turn without starting a
// new one. expectedTurnID must match the active turn id.
func (c *Client) SteerTurn(ctx context.Context, threadID, expectedTurnID string, input []InputItem) (string, error) {
	params := SteerTurnParams{ThreadID: threadID, Input: input, ExpectedTurnID: expectedTurnID}
	var result SteerTurnResult
	if err := c.call(ctx, "turn/steer", params, &result); err != nil {
		return "", err
	}
	return result.TurnID, nil
}

// InterruptTurn requests cancellation of an in-flight turn. On success the
// turn ends with status "interrupted".
func (c *Client) InterruptTurn(ctx context.Context, threadID, turnID string) error {
	return c.call(ctx, "turn/interrupt", InterruptTurnParams{ThreadID: threadID, TurnID: turnID}, nil)
}

// CompactThread triggers manual history compaction. Progress streams as
// ordinary turn and item notifications.
func (c *Client) CompactThread(ctx context.Context, threadID string) error {
	return c.call(ctx, "thread/compact/start", ThreadIDParams{ThreadID: threadID}, nil)
}

// RunShellCommand runs a user-initiated shell command against a thread. It
// runs outside the sandbox with full access.
func (c *Client) RunShellCommand(ctx context.Context, threadID, command string) error {
	params := struct {
		ThreadID string `json:"threadId"`
		Command  string `json:"command"`
	}{ThreadID: threadID, Command: command}
	return c.call(ctx, "thread/shellCommand", params, nil)
}
