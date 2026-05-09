// Package events provides a simple in-process event bus for the Mulwiki server.
// It follows the Multica pattern: typed events with workspace/agent/task scoping
// so that subscribers (e.g. the WebSocket hub) can route messages efficiently.
package events

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// EventType identifies the kind of event.
type EventType string

const (
	EventTaskDispatched EventType = "task.dispatched"
	EventTaskStarted    EventType = "task.started"
	EventTaskCompleted  EventType = "task.completed"
	EventTaskFailed     EventType = "task.failed"
	EventTaskCancelled  EventType = "task.cancelled"
	EventTaskMessage    EventType = "task.message"
	EventDaemonOnline   EventType = "daemon.online"
	EventDaemonOffline  EventType = "daemon.offline"
	EventAgentUpdated   EventType = "agent.updated"
	EventSchemaUpdated  EventType = "schema.updated"
	EventSourceUpdated  EventType = "source.updated"
	EventWikiUpdated    EventType = "wiki.updated"
)

// Event is a single event delivered through the bus.
type Event struct {
	Type        EventType `json:"type"`
	WorkspaceID string    `json:"workspace_id"`
	AgentID     string    `json:"agent_id"`
	TaskID      string    `json:"task_id"`
	Payload     any       `json:"payload"`
	Timestamp   time.Time `json:"timestamp"`
}

// Subscriber is a function that receives events.
type Subscriber func(Event)

type subscriberEntry struct {
	fn Subscriber
	id int64
}

var nextSubID atomic.Int64

// Bus is a simple in-process publish/subscribe event bus.
// Subscribers are registered per event type and notified synchronously
// in the calling goroutine (Publish blocks until all subscribers return).
type Bus struct {
	mu          sync.RWMutex
	subscribers map[EventType][]subscriberEntry
}

// NewBus creates a new EventBus.
func NewBus() *Bus {
	return &Bus{
		subscribers: make(map[EventType][]subscriberEntry),
	}
}

// Subscribe registers a subscriber for a specific event type.
// It returns an unsubscribe function for convenience.
func (b *Bus) Subscribe(eventType EventType, subscriber Subscriber) func() {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := nextSubID.Add(1)
	b.subscribers[eventType] = append(b.subscribers[eventType], subscriberEntry{fn: subscriber, id: id})
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		entries := b.subscribers[eventType]
		for i, e := range entries {
			if e.id == id {
				b.subscribers[eventType] = append(entries[:i], entries[i+1:]...)
				return
			}
		}
	}
}

// Publish delivers an event to all subscribers of its type.
// Subscribers are called synchronously; slow subscribers block the caller.
func (b *Bus) Publish(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	b.mu.RLock()
	entries := append([]subscriberEntry(nil), b.subscribers[event.Type]...)
	b.mu.RUnlock()

	for _, entry := range entries {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("event subscriber panicked",
						"event_type", event.Type,
						"workspace_id", event.WorkspaceID,
						"agent_id", event.AgentID,
						"panic", r,
					)
				}
			}()
			entry.fn(event)
		}()
	}
}
