// Package realtime provides a WebSocket hub that follows the Multica pattern:
// clients connect with workspace + optional agent scope, and the hub broadcasts
// events from the internal event bus to connected clients in non-blocking fashion.
package realtime

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/tethy/mulwiki/server/internal/events"
)

const (
	// writeBuffer is the per-client output channel capacity.
	// When full, the client is evicted (slow consumer).
	writeBuffer = 256

	// pingInterval is how often we send WebSocket pings.
	pingInterval = 25 * time.Second

	// pongTimeout is how long we wait for a pong response before closing.
	pongTimeout = 60 * time.Second

	// maxMessageSize is the largest message we're willing to read (64KB).
	maxMessageSize = 65536
)

// ScopeType identifies the subscription scope for a connected client.
type ScopeType string

const (
	ScopeWorkspace ScopeType = "workspace"
	ScopeAgent     ScopeType = "agent"
)

// Client represents a single WebSocket connection.
type Client struct {
	conn   *websocket.Conn
	send   chan []byte
	scopes map[string]struct{} // "workspace:ws1", "agent:agent1"
	hub    *Hub
	done   chan struct{}
	once   sync.Once
}

// Hub manages all WebSocket connections and routes broadcast
// messages to clients based on their scope subscriptions.
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}

	// Event bus integration — the hub subscribes on startup.
	bus *events.Bus

	upgrader websocket.Upgrader
}

// NewHub creates a Hub and subscribes it to the given event bus.
func NewHub(bus *events.Bus) *Hub {
	h := &Hub{
		clients: make(map[*Client]struct{}),
		bus:     bus,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				return true // allowed origins are handled by the CORS middleware
			},
		},
	}
	h.subscribeToBus()
	return h
}

// subscribeToBus wires the hub into the event bus so that events are
// automatically forwarded to the appropriate WebSocket rooms.
func (h *Hub) subscribeToBus() {
	if h.bus == nil {
		return
	}
	// Forward all known event types to their scoped rooms.
	allTypes := []events.EventType{
		events.EventTaskDispatched,
		events.EventTaskStarted,
		events.EventTaskCompleted,
		events.EventTaskFailed,
		events.EventTaskCancelled,
		events.EventTaskMessage,
		events.EventDaemonOnline,
		events.EventDaemonOffline,
		events.EventAgentUpdated,
		events.EventSchemaUpdated,
		events.EventSourceUpdated,
		events.EventWikiUpdated,
	}
	for _, t := range allTypes {
		h.bus.Subscribe(t, func(ev events.Event) {
			msg, err := json.Marshal(ev)
			if err != nil {
				slog.Error("realtime: failed to marshal event", "type", ev.Type, "error", err)
				return
			}
			if ev.WorkspaceID != "" {
				h.BroadcastToScope(ScopeWorkspace, ev.WorkspaceID, msg)
			}
			if ev.AgentID != "" {
				h.BroadcastToScope(ScopeAgent, ev.AgentID, msg)
			}
		})
	}
}

// ServeWS upgrades an HTTP request to a WebSocket connection.
// Query params:
//   - workspace     (required) — subscribe to workspace:{id}
//   - workspace_id  (legacy alias)
//   - agent_id      (optional) — also subscribe to agent:{id}
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspace")
	if workspaceID == "" {
		workspaceID = r.URL.Query().Get("workspace_id")
	}
	if workspaceID == "" {
		http.Error(w, "workspace query parameter is required", http.StatusBadRequest)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("realtime: websocket upgrade failed", "error", err)
		return
	}

	client := &Client{
		conn:   conn,
		send:   make(chan []byte, writeBuffer),
		scopes: make(map[string]struct{}),
		hub:    h,
		done:   make(chan struct{}),
	}

	client.scopes[scopeKey(ScopeWorkspace, workspaceID)] = struct{}{}
	if agentID := r.URL.Query().Get("agent_id"); agentID != "" {
		client.scopes[scopeKey(ScopeAgent, agentID)] = struct{}{}
	}

	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()

	slog.Info("realtime: client connected",
		"workspace_id", workspaceID,
		"agent_id", r.URL.Query().Get("agent_id"),
		"total_clients", len(h.clients),
	)

	go client.writePump()
	go client.readPump()
}

// BroadcastToScope sends a message to all clients subscribed to a scope.
// Message delivery is non-blocking — slow clients are evicted.
func (h *Hub) BroadcastToScope(scopeType ScopeType, scopeID string, message []byte) {
	key := scopeKey(scopeType, scopeID)

	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		if _, ok := c.scopes[key]; ok {
			select {
			case c.send <- message:
			default:
				// Slow consumer — evict.
				go c.close()
			}
		}
	}
}

// writePump pumps messages from the send channel to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				// Channel closed — hub told us to stop.
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				slog.Warn("realtime: write failed, evicting client", "error", err)
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-c.done:
			return
		}
	}
}

// readPump reads messages from the WebSocket connection.
// We read to detect disconnects and consume control messages.
func (c *Client) readPump() {
	defer c.close()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongTimeout))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongTimeout))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Warn("realtime: read error", "error", err)
			}
			return
		}
		// We don't process incoming messages from the client.
		// In the future this could be extended for client→server messaging.
	}
}

// close removes the client from the hub and closes the connection.
func (c *Client) close() {
	c.once.Do(func() {
		close(c.done)
		c.hub.mu.Lock()
		delete(c.hub.clients, c)
		c.hub.mu.Unlock()
		if c.conn != nil {
			c.conn.Close()
		}
		slog.Info("realtime: client disconnected", "total_clients", len(c.hub.clients))
	})
}

// scopeKey builds the internal map key for a scope subscription.
func scopeKey(scopeType ScopeType, scopeID string) string {
	return string(scopeType) + ":" + scopeID
}
