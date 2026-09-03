package truenas

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
)

// Subscription is a stable handle whose server ID is replaced after reconnect.
type Subscription struct {
	event string
	mu    sync.RWMutex
	id    string
}

func (s *Subscription) Event() string { return s.event }
func (s *Subscription) ID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.id
}
func (s *Subscription) setID(id string) {
	s.mu.Lock()
	s.id = id
	s.mu.Unlock()
}

// Subscribe calls core.subscribe and registers the event for restoration after
// reconnect. The returned handle remains valid when its server ID changes.
func (c *Client) Subscribe(ctx context.Context, event string) (*Subscription, error) {
	event = strings.TrimSpace(event)
	if event == "" {
		return nil, &ValidationError{Field: "event", Message: "is required"}
	}
	var id string
	if err := c.Call(ctx, "core.subscribe", []any{event}, &id); err != nil {
		return nil, err
	}
	subscription := &Subscription{event: event, id: id}
	c.subscriptionsMu.Lock()
	c.subscriptions[subscription] = struct{}{}
	c.subscriptionsMu.Unlock()
	return subscription, nil
}

// Unsubscribe calls core.unsubscribe with the current server-side ID and stops
// restoring the subscription on future reconnects.
func (c *Client) Unsubscribe(ctx context.Context, subscription *Subscription) error {
	if subscription == nil {
		return &ValidationError{Field: "subscription", Message: "is required"}
	}
	c.subscriptionsMu.Lock()
	_, exists := c.subscriptions[subscription]
	if exists {
		delete(c.subscriptions, subscription)
	}
	c.subscriptionsMu.Unlock()
	if !exists {
		return errors.New("truenas: subscription is not registered")
	}
	return c.Call(ctx, "core.unsubscribe", []any{subscription.ID()}, nil)
}

func (c *Client) restoreSubscriptions(ctx context.Context) error {
	c.subscriptionsMu.Lock()
	subscriptions := make([]*Subscription, 0, len(c.subscriptions))
	for subscription := range c.subscriptions {
		subscriptions = append(subscriptions, subscription)
	}
	c.subscriptionsMu.Unlock()
	for _, subscription := range subscriptions {
		var id string
		if err := c.callOnceConnected(ctx, "core.subscribe", []any{subscription.Event()}, &id); err != nil {
			return err
		}
		subscription.setID(id)
	}
	return nil
}

// DecodeNotification decodes notification parameters into a typed value.
func DecodeNotification[T any](notification Notification) (T, error) {
	var value T
	err := json.Unmarshal(notification.Params, &value)
	return value, err
}

// CollectionUpdate is the common payload sent by query collection events.
type CollectionUpdate struct {
	Collection string          `json:"collection"`
	Message    string          `json:"msg"`
	ID         json.RawMessage `json:"id,omitempty"`
	Fields     json.RawMessage `json:"fields,omitempty"`
	Extra      json.RawMessage `json:"extra,omitempty"`
}
