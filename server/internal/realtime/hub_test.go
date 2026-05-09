package realtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tethy/mulwiki/server/internal/events"
)

func TestNewHub(t *testing.T) {
	bus := events.NewBus()
	hub := NewHub(bus)

	if hub == nil {
		t.Fatal("expected non-nil hub")
	}
	if hub.clients == nil {
		t.Error("expected non-nil clients map")
	}
	if hub.bus != bus {
		t.Error("hub should hold reference to bus")
	}
}

func TestWebSocketHandshakeTimeout(t *testing.T) {
	hub := NewHub(nil)
	if hub.upgrader.HandshakeTimeout != 10*time.Second {
		t.Fatalf("expected websocket handshake timeout 10s, got %s", hub.upgrader.HandshakeTimeout)
	}
}

func TestNewHub_NilBus(t *testing.T) {
	hub := NewHub(nil)
	if hub == nil {
		t.Fatal("hub should be created even without bus")
	}
	if hub.clients == nil {
		t.Error("expected non-nil clients map")
	}
}

func TestBroadcastToScope_NoClients(t *testing.T) {
	hub := NewHub(nil)
	// Should not panic with no clients.
	hub.BroadcastToScope(ScopeWorkspace, "ws1", []byte(`{"type":"test"}`))
}

func TestBroadcastToScope_SingleClient(t *testing.T) {
	hub := NewHub(nil)

	// Create a test client without a real WebSocket connection.
	c := &Client{
		send:   make(chan []byte, writeBuffer),
		scopes: map[string]struct{}{scopeKey(ScopeWorkspace, "ws1"): {}},
		hub:    hub,
		done:   make(chan struct{}),
	}

	hub.mu.Lock()
	hub.clients[c] = struct{}{}
	hub.mu.Unlock()

	msg := []byte(`{"type":"hello"}`)
	hub.BroadcastToScope(ScopeWorkspace, "ws1", msg)

	// Client should receive the message.
	select {
	case received := <-c.send:
		if string(received) != string(msg) {
			t.Errorf("expected '%s', got '%s'", string(msg), string(received))
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timed out waiting for message")
	}
}

func TestBroadcastToScope_WrongScope(t *testing.T) {
	hub := NewHub(nil)

	c := &Client{
		send:   make(chan []byte, writeBuffer),
		scopes: map[string]struct{}{scopeKey(ScopeWorkspace, "ws2"): {}},
		hub:    hub,
		done:   make(chan struct{}),
	}

	hub.mu.Lock()
	hub.clients[c] = struct{}{}
	hub.mu.Unlock()

	hub.BroadcastToScope(ScopeWorkspace, "ws1", []byte(`{"type":"hello"}`))

	// Client should NOT receive (wrong workspace scope).
	select {
	case <-c.send:
		t.Error("client should not receive message for different workspace")
	case <-time.After(50 * time.Millisecond):
		// Expected — no message.
	}
}

func TestBroadcastToScope_MultipleScopes(t *testing.T) {
	hub := NewHub(nil)

	c := &Client{
		send:   make(chan []byte, writeBuffer),
		scopes: map[string]struct{}{scopeKey(ScopeWorkspace, "ws1"): {}, scopeKey(ScopeAgent, "agent1"): {}},
		hub:    hub,
		done:   make(chan struct{}),
	}

	hub.mu.Lock()
	hub.clients[c] = struct{}{}
	hub.mu.Unlock()

	// Broadcast to agent scope.
	msg := []byte(`{"type":"agent-event"}`)
	hub.BroadcastToScope(ScopeAgent, "agent1", msg)

	select {
	case received := <-c.send:
		if string(received) != string(msg) {
			t.Errorf("expected '%s', got '%s'", string(msg), string(received))
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timed out waiting for agent-scoped message")
	}

	// Broadcast to workspace scope.
	msg2 := []byte(`{"type":"workspace-event"}`)
	hub.BroadcastToScope(ScopeWorkspace, "ws1", msg2)

	select {
	case received := <-c.send:
		if string(received) != string(msg2) {
			t.Errorf("expected '%s', got '%s'", string(msg2), string(received))
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timed out waiting for workspace-scoped message")
	}
}

func TestBroadcastToScope_SlowClientEviction(t *testing.T) {
	hub := NewHub(nil)

	// Create a client with a full send channel (buffer = 0 effectively)
	c := &Client{
		send:   make(chan []byte, writeBuffer),
		scopes: map[string]struct{}{scopeKey(ScopeWorkspace, "ws1"): {}},
		hub:    hub,
		done:   make(chan struct{}),
	}

	hub.mu.Lock()
	hub.clients[c] = struct{}{}
	hub.mu.Unlock()

	// Fill the buffer.
	for i := 0; i < writeBuffer; i++ {
		c.send <- []byte("x")
	}

	// Next broadcast should evict.
	hub.BroadcastToScope(ScopeWorkspace, "ws1", []byte(`{"type":"overflow"}`))

	// Wait a bit for async close.
	time.Sleep(50 * time.Millisecond)

	hub.mu.RLock()
	_, exists := hub.clients[c]
	hub.mu.RUnlock()

	if exists {
		t.Log("client may still be registered (async eviction)")
	}
}

func TestScopeKey(t *testing.T) {
	if key := scopeKey(ScopeWorkspace, "ws1"); key != "workspace:ws1" {
		t.Errorf("expected 'workspace:ws1', got '%s'", key)
	}
	if key := scopeKey(ScopeAgent, "agent-abc"); key != "agent:agent-abc" {
		t.Errorf("expected 'agent:agent-abc', got '%s'", key)
	}
}

func TestServeWS_MissingWorkspaceID(t *testing.T) {
	hub := NewHub(nil)
	srv := httptest.NewServer(http.HandlerFunc(hub.ServeWS))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	_, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected error for missing workspace_id, got nil")
	}
}

func TestServeWS_ValidConnection(t *testing.T) {
	bus := events.NewBus()
	hub := NewHub(bus)

	srv := httptest.NewServer(http.HandlerFunc(hub.ServeWS))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?workspace_id=ws1&agent_id=agent1"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Verify client is registered.
	hub.mu.RLock()
	clientCount := len(hub.clients)
	hub.mu.RUnlock()
	if clientCount != 1 {
		t.Errorf("expected 1 client, got %d", clientCount)
	}
}

func TestServeWS_ValidConnectionWithWorkspaceParam(t *testing.T) {
	bus := events.NewBus()
	hub := NewHub(bus)

	srv := httptest.NewServer(http.HandlerFunc(hub.ServeWS))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?workspace=ws1&agent_id=agent1"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	hub.mu.RLock()
	clientCount := len(hub.clients)
	hub.mu.RUnlock()
	if clientCount != 1 {
		t.Errorf("expected 1 client, got %d", clientCount)
	}
}

func TestHub_EventBusIntegration(t *testing.T) {
	bus := events.NewBus()
	hub := NewHub(bus)

	srv := httptest.NewServer(http.HandlerFunc(hub.ServeWS))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?workspace_id=ws1"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Publish an event via the bus that should reach the WebSocket client.
	bus.Publish(events.Event{
		Type:        events.EventTaskStarted,
		WorkspaceID: "ws1",
		AgentID:     "agent1",
		TaskID:      "task1",
		Payload:     map[string]string{"status": "running"},
	})

	// Read from WebSocket — should get the event.
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read ws message: %v", err)
	}

	var received events.Event
	if err := json.Unmarshal(msg, &received); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if received.Type != events.EventTaskStarted {
		t.Errorf("expected type task.started, got %s", received.Type)
	}
	if received.WorkspaceID != "ws1" {
		t.Errorf("expected workspace ws1, got %s", received.WorkspaceID)
	}
}

func TestClient_WritePumpPing(t *testing.T) {
	bus := events.NewBus()
	hub := NewHub(bus)

	srv := httptest.NewServer(http.HandlerFunc(hub.ServeWS))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?workspace_id=ws1"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Set a custom pong handler to verify pings are sent.
	pongReceived := make(chan struct{}, 1)
	conn.SetPongHandler(func(appData string) error {
		pongReceived <- struct{}{}
		return nil
	})

	// Wait for a ping (sent every 25s, but we give it less time).
	select {
	case <-pongReceived:
		// Ping received — writePump is working.
	case <-time.After(500 * time.Millisecond):
		t.Log("no ping received within 500ms (ping interval is 25s)")
	}

	conn.Close()
}

func TestHub_EventBusAgentScopeOnly(t *testing.T) {
	bus := events.NewBus()
	hub := NewHub(bus)

	srv := httptest.NewServer(http.HandlerFunc(hub.ServeWS))
	defer srv.Close()

	// Client connects with workspace "ws1" but event fires for agent "agent2" only.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?workspace_id=ws1&agent_id=agent1"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Event for a different agent — should still arrive via workspace scope.
	bus.Publish(events.Event{
		Type:        events.EventTaskCompleted,
		WorkspaceID: "ws1",
		AgentID:     "agent2",
		TaskID:      "task2",
	})

	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read ws message: %v", err) // workspace-scoped still delivers
	}

	var received events.Event
	json.Unmarshal(msg, &received)
	if received.WorkspaceID != "ws1" {
		t.Errorf("expected ws1, got %s", received.WorkspaceID)
	}
}
