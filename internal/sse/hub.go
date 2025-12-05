package sse

import (
	"encoding/json"
	"sync"
)

// EventType represents the type of SSE event
type EventType string

// Resource change events - *:changed for creates/updates, *:deleted for deletions
const (
	EventGroupsChanged       EventType = "groups:changed"
	EventGroupsDeleted       EventType = "groups:deleted"
	EventRolesChanged        EventType = "roles:changed"
	EventRolesDeleted        EventType = "roles:deleted"
	EventTemplatesChanged    EventType = "templates:changed"
	EventTemplatesDeleted    EventType = "templates:deleted"
	EventTemplateVarsChanged EventType = "templatevars:changed"
	EventTemplateVarsDeleted EventType = "templatevars:deleted"
	EventUsersChanged        EventType = "users:changed"
	EventUsersDeleted        EventType = "users:deleted"
	EventTokensChanged       EventType = "tokens:changed"
	EventTokensDeleted       EventType = "tokens:deleted"
	EventVolumesChanged      EventType = "volumes:changed"
	EventVolumesDeleted      EventType = "volumes:deleted"
	EventSessionsChanged     EventType = "sessions:changed"
	EventSessionsDeleted     EventType = "sessions:deleted"
	EventTunnelsChanged      EventType = "tunnels:changed"
	EventTunnelsDeleted      EventType = "tunnels:deleted"
	EventAuditLogsChanged    EventType = "auditlogs:changed"

	// Space events for frequently changing data
	EventSpaceChanged EventType = "space:changed"
	EventSpaceDeleted EventType = "space:deleted"

	// Authentication events
	EventAuthRequired EventType = "auth:required"
)

// Event represents an SSE event to be sent to clients
type Event struct {
	Type    EventType   `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

// ResourcePayload contains data for resource-specific events
type ResourcePayload struct {
	Id               string `json:"id"`
	UserId           string `json:"user_id,omitempty"`
	SharedWithUserId string `json:"shared_with_user_id,omitempty"`
	PreviousUserId   string `json:"previous_user_id,omitempty"`
}

// Client represents a connected SSE client
type Client struct {
	userId    string
	sessionId string
	send      chan []byte
	hub       *Hub
}

// Hub manages all SSE client connections and event broadcasting
type Hub struct {
	// Registered clients by user ID for targeted events
	clients map[*Client]bool
	// Channel for broadcasting events to all clients
	broadcast chan *Event
	// Channel for registering new clients
	register chan *Client
	// Channel for unregistering clients
	unregister chan *Client
	// Channel for shutdown signal
	shutdown chan struct{}
	// Mutex for thread-safe operations
	mu sync.RWMutex
}

var (
	globalHub  *Hub
	hubOnce    sync.Once
	hubStarted bool
)

// GetHub returns the singleton SSE hub instance
func GetHub() *Hub {
	hubOnce.Do(func() {
		globalHub = &Hub{
			clients:    make(map[*Client]bool),
			broadcast:  make(chan *Event, 256),
			register:   make(chan *Client),
			unregister: make(chan *Client),
			shutdown:   make(chan struct{}),
		}
	})
	return globalHub
}

// Start begins the hub's event processing loop
func (h *Hub) Start() {
	if hubStarted {
		return
	}
	hubStarted = true
	go h.run()
}

// run is the main event loop for the hub
func (h *Hub) run() {
	for {
		select {
		case <-h.shutdown:
			// Close all client connections
			h.mu.Lock()
			for client := range h.clients {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			return
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
		case event := <-h.broadcast:
			h.mu.RLock()
			data, err := json.Marshal(event)
			if err != nil {
				h.mu.RUnlock()
				continue
			}
			for client := range h.clients {
				select {
				case client.send <- data:
				default:
					// Client buffer is full, remove them
					h.mu.RUnlock()
					h.mu.Lock()
					delete(h.clients, client)
					close(client.send)
					h.mu.Unlock()
					h.mu.RLock()
				}
			}
			h.mu.RUnlock()
		}
	}
}

// NewClient creates a new SSE client and registers it with the hub
func (h *Hub) NewClient(userId, sessionId string) *Client {
	client := &Client{
		userId:    userId,
		sessionId: sessionId,
		send:      make(chan []byte, 64),
		hub:       h,
	}
	h.register <- client
	return client
}

// Close unregisters a client from the hub
func (c *Client) Close() {
	c.hub.unregister <- c
}

// Send returns the channel for receiving events
func (c *Client) Send() <-chan []byte {
	return c.send
}

// Broadcast sends an event to all connected clients
func (h *Hub) Broadcast(event *Event) {
	select {
	case h.broadcast <- event:
	default:
		// Broadcast channel is full, drop the event
	}
}

// BroadcastToUser sends an event to all clients for a specific user
func (h *Hub) BroadcastToUser(userId string, event *Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	for client := range h.clients {
		if client.userId == userId {
			select {
			case client.send <- data:
			default:
				// Client buffer is full
			}
		}
	}
	h.mu.RUnlock()
}

// InvalidateSession sends an auth required event to all clients with a specific session
func (h *Hub) InvalidateSession(sessionId string) {
	event := &Event{
		Type: EventAuthRequired,
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	for client := range h.clients {
		if client.sessionId == sessionId {
			select {
			case client.send <- data:
			default:
			}
		}
	}
	h.mu.RUnlock()
}

// ClientCount returns the number of connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Shutdown closes all client connections and stops the hub
func (h *Hub) Shutdown() {
	if hubStarted {
		close(h.shutdown)
		hubStarted = false
	}
}
