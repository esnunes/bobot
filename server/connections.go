// server/connections.go
package server

import (
	"sync"

	"github.com/gorilla/websocket"
)

// WebSocketWriter is the interface for writing to a WebSocket connection.
// This allows for easier testing with mock connections.
type WebSocketWriter interface {
	WriteMessage(messageType int, data []byte) error
	Close() error
}

// ConnectionRegistry manages WebSocket connections per user for multi-device support.
type ConnectionRegistry struct {
	mu    sync.RWMutex
	conns map[int64][]WebSocketWriter
}

// NewConnectionRegistry creates a new ConnectionRegistry.
func NewConnectionRegistry() *ConnectionRegistry {
	return &ConnectionRegistry{
		conns: make(map[int64][]WebSocketWriter),
	}
}

// Add registers a connection for a user.
func (r *ConnectionRegistry) Add(userID int64, conn WebSocketWriter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conns[userID] = append(r.conns[userID], conn)
}

// Remove unregisters a connection for a user.
func (r *ConnectionRegistry) Remove(userID int64, conn WebSocketWriter) {
	r.mu.Lock()
	defer r.mu.Unlock()

	conns := r.conns[userID]
	for i, c := range conns {
		if c == conn {
			r.conns[userID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}

	if len(r.conns[userID]) == 0 {
		delete(r.conns, userID)
	}
}

// Broadcast sends a message to all connections for a user.
func (r *ConnectionRegistry) Broadcast(userID int64, data []byte) {
	r.mu.RLock()
	conns := r.conns[userID]
	r.mu.RUnlock()

	for _, conn := range conns {
		conn.WriteMessage(websocket.TextMessage, data)
	}
}

// Count returns the number of connections for a user.
func (r *ConnectionRegistry) Count(userID int64) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.conns[userID])
}
