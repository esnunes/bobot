// server/connections_test.go
package server

import (
	"sync"
	"testing"
)

type mockConn struct {
	id       int
	messages [][]byte
	mu       sync.Mutex
	closed   bool
}

func (m *mockConn) WriteMessage(messageType int, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.messages = append(m.messages, data)
	return nil
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func TestConnectionRegistry_AddRemove(t *testing.T) {
	registry := NewConnectionRegistry()

	conn1 := &mockConn{id: 1}
	conn2 := &mockConn{id: 2}

	registry.Add(1, conn1)
	registry.Add(1, conn2)
	registry.Add(2, conn1) // Different user

	if registry.Count(1) != 2 {
		t.Errorf("expected 2 connections for user 1, got %d", registry.Count(1))
	}
	if registry.Count(2) != 1 {
		t.Errorf("expected 1 connection for user 2, got %d", registry.Count(2))
	}

	registry.Remove(1, conn1)
	if registry.Count(1) != 1 {
		t.Errorf("expected 1 connection after remove, got %d", registry.Count(1))
	}
}

func TestConnectionRegistry_Broadcast(t *testing.T) {
	registry := NewConnectionRegistry()

	conn1 := &mockConn{id: 1}
	conn2 := &mockConn{id: 2}
	conn3 := &mockConn{id: 3} // Different user

	registry.Add(1, conn1)
	registry.Add(1, conn2)
	registry.Add(2, conn3)

	registry.Broadcast(1, []byte("hello"))

	if len(conn1.messages) != 1 {
		t.Errorf("conn1 should have 1 message, got %d", len(conn1.messages))
	}
	if len(conn2.messages) != 1 {
		t.Errorf("conn2 should have 1 message, got %d", len(conn2.messages))
	}
	if len(conn3.messages) != 0 {
		t.Errorf("conn3 should have 0 messages, got %d", len(conn3.messages))
	}
}
