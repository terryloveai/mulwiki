package events

import (
	"sync"
	"testing"
	"time"
)

func TestNewBus(t *testing.T) {
	bus := NewBus()
	if bus == nil {
		t.Fatal("expected non-nil bus")
	}
	if bus.subscribers == nil {
		t.Error("expected non-nil subscribers map")
	}
}

func TestSubscribeAndPublish(t *testing.T) {
	bus := NewBus()
	var received []Event
	var mu sync.Mutex

	unsub := bus.Subscribe(EventTaskDispatched, func(e Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	bus.Publish(Event{
		Type:        EventTaskDispatched,
		WorkspaceID: "ws1",
		AgentID:     "agent1",
		TaskID:      "task1",
		Payload:     map[string]string{"key": "value"},
	})

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}

	e := received[0]
	if e.Type != EventTaskDispatched {
		t.Errorf("expected type %s, got %s", EventTaskDispatched, e.Type)
	}
	if e.WorkspaceID != "ws1" {
		t.Errorf("expected workspace ws1, got %s", e.WorkspaceID)
	}
	if e.AgentID != "agent1" {
		t.Errorf("expected agent agent1, got %s", e.AgentID)
	}
	if e.TaskID != "task1" {
		t.Errorf("expected task task1, got %s", e.TaskID)
	}

	// Verify unsubscribe works.
	unsub()
	bus.Publish(Event{Type: EventTaskDispatched, WorkspaceID: "ws2"})
	if len(received) != 1 {
		t.Errorf("expected 1 event after unsubscribe, got %d", len(received))
	}
}

func TestPublish_TimestampAutoSet(t *testing.T) {
	bus := NewBus()
	var ts time.Time

	bus.Subscribe(EventTaskStarted, func(e Event) {
		ts = e.Timestamp
	})

	before := time.Now().UTC()
	bus.Publish(Event{Type: EventTaskStarted, WorkspaceID: "ws1"})
	after := time.Now().UTC()

	if ts.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %s not between %s and %s", ts, before, after)
	}
}

func TestPublish_TimestampPreserved(t *testing.T) {
	bus := NewBus()
	customTS := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	var ts time.Time
	bus.Subscribe(EventTaskCompleted, func(e Event) {
		ts = e.Timestamp
	})

	bus.Publish(Event{Type: EventTaskCompleted, WorkspaceID: "ws1", Timestamp: customTS})
	if !ts.Equal(customTS) {
		t.Errorf("expected %v, got %v", customTS, ts)
	}
}

func TestPublish_NoSubscribers(t *testing.T) {
	bus := NewBus()
	// Should not panic when no subscribers exist.
	bus.Publish(Event{Type: EventTaskDispatched, WorkspaceID: "ws1"})
}

func TestPublish_MultipleSubscribers(t *testing.T) {
	bus := NewBus()
	var count int
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		bus.Subscribe(EventDaemonOnline, func(e Event) {
			mu.Lock()
			count++
			mu.Unlock()
		})
	}

	bus.Publish(Event{Type: EventDaemonOnline, WorkspaceID: "ws1"})
	if count != 5 {
		t.Errorf("expected 5 subscribers called, got %d", count)
	}
}

func TestPublish_SubscriberPanicRecovery(t *testing.T) {
	bus := NewBus()
	var secondCalled bool

	// First subscriber panics.
	bus.Subscribe(EventTaskFailed, func(e Event) {
		panic("test panic")
	})
	// Second subscriber should still be called.
	bus.Subscribe(EventTaskFailed, func(e Event) {
		secondCalled = true
	})

	// Should not panic the caller.
	bus.Publish(Event{Type: EventTaskFailed, WorkspaceID: "ws1"})
	if !secondCalled {
		t.Error("second subscriber should still be called after panic")
	}
}

func TestPublish_OnlyMatchingType(t *testing.T) {
	bus := NewBus()
	var called bool

	bus.Subscribe(EventTaskDispatched, func(e Event) {
		called = true
	})

	// Publish a different type — subscriber should NOT be called.
	bus.Publish(Event{Type: EventTaskCompleted, WorkspaceID: "ws1"})
	if called {
		t.Error("subscriber should not be called for non-matching event type")
	}
}

func TestAllEventTypes(t *testing.T) {
	types := []EventType{
		EventTaskDispatched,
		EventTaskStarted,
		EventTaskCompleted,
		EventTaskFailed,
		EventTaskCancelled,
		EventTaskMessage,
		EventDaemonOnline,
		EventDaemonOffline,
		EventAgentUpdated,
		EventSchemaUpdated,
		EventSourceUpdated,
		EventWikiUpdated,
	}

	for _, et := range types {
		bus := NewBus()
		called := false
		bus.Subscribe(et, func(e Event) {
			called = true
		})
		bus.Publish(Event{Type: et, WorkspaceID: "ws1"})
		if !called {
			t.Errorf("event type %s did not fire subscriber", et)
		}
	}
}

func TestSubscribeReturnsUnsubscribeMultiple(t *testing.T) {
	bus := NewBus()
	var count int
	var mu sync.Mutex

	unsub1 := bus.Subscribe(EventTaskDispatched, func(e Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})
	unsub2 := bus.Subscribe(EventTaskDispatched, func(e Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	bus.Publish(Event{Type: EventTaskDispatched})
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}

	unsub1()
	count = 0
	bus.Publish(Event{Type: EventTaskDispatched})
	if count != 1 {
		t.Errorf("expected 1 after one unsubscribe, got %d", count)
	}

	unsub2()
	count = 0
	bus.Publish(Event{Type: EventTaskDispatched})
	if count != 0 {
		t.Errorf("expected 0 after both unsubscribed, got %d", count)
	}
}
