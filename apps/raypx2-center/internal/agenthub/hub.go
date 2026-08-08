package agenthub

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/protocol"
)

var ErrStaleRegistration = errors.New("stale agent registration")

type Conn interface {
	ID() string
	SessionID() string
	Send(context.Context, protocol.Frame) error
	Close(reason string) error
}

type Option func(*Hub)

func WithSessionRevoker(revoke func(string) error) Option {
	return func(h *Hub) { h.revokeSession = revoke }
}

type Hub struct {
	mu            sync.RWMutex
	connections   map[string]Conn
	pending       map[string]pendingResponse
	epochs        map[string]uint64
	revokeSession func(string) error
}

func New(options ...Option) *Hub {
	h := &Hub{
		connections: make(map[string]Conn),
		pending:     make(map[string]pendingResponse),
		epochs:      make(map[string]uint64),
	}
	for _, option := range options {
		option(h)
	}
	return h
}

func (h *Hub) Register(nodeKey string, conn Conn) (replaced bool) {
	h.mu.Lock()
	old := h.connections[nodeKey]
	h.connections[nodeKey] = conn
	h.mu.Unlock()
	h.closeReplaced(old, conn)
	return old != nil && old.ID() != conn.ID()
}

// Epoch returns the current registration generation for a node.
func (h *Hub) Epoch(nodeKey string) uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.epochs[nodeKey]
}

// RegisterAtEpoch registers a connection only if no intervening Kick occurred.
func (h *Hub) RegisterAtEpoch(nodeKey string, epoch uint64, conn Conn) (bool, error) {
	h.mu.Lock()
	if h.epochs[nodeKey] != epoch {
		h.mu.Unlock()
		return false, ErrStaleRegistration
	}
	old := h.connections[nodeKey]
	h.connections[nodeKey] = conn
	h.mu.Unlock()
	h.closeReplaced(old, conn)
	return old != nil && old.ID() != conn.ID(), nil
}

func (h *Hub) closeReplaced(old, conn Conn) {
	if old == nil || old.ID() == conn.ID() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	payload, _ := json.Marshal(map[string]string{"reason": "replaced"})
	_ = old.Send(ctx, protocol.Frame{
		Type:    "bye",
		TS:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload: payload,
	})
	_ = old.Close("replaced")
	if h.revokeSession != nil && old.SessionID() != "" && old.SessionID() != conn.SessionID() {
		_ = h.revokeSession(old.SessionID())
	}
}

func (h *Hub) Unregister(nodeKey, connID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if current := h.connections[nodeKey]; current != nil && current.ID() == connID {
		delete(h.connections, nodeKey)
	}
}

func (h *Hub) IsCurrent(nodeKey, connID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	current := h.connections[nodeKey]
	return current != nil && current.ID() == connID
}

func (h *Hub) HasConnection(nodeKey string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.connections[nodeKey]
	return ok
}

// Kick sends a terminal bye frame, closes the node connection, and revokes its session.
func (h *Hub) Kick(nodeKey, reason string) bool {
	h.mu.Lock()
	h.epochs[nodeKey]++
	conn := h.connections[nodeKey]
	if conn != nil {
		delete(h.connections, nodeKey)
	}
	h.mu.Unlock()
	if conn == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	payload, _ := json.Marshal(map[string]string{"reason": reason})
	_ = conn.Send(ctx, protocol.Frame{
		Type:    "bye",
		TS:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload: payload,
	})
	_ = conn.Close(reason)
	if h.revokeSession != nil && conn.SessionID() != "" {
		_ = h.revokeSession(conn.SessionID())
	}
	return true
}

func (h *Hub) Send(nodeKey string, frame protocol.Frame) error {
	h.mu.RLock()
	conn := h.connections[nodeKey]
	h.mu.RUnlock()
	if conn == nil {
		return ErrNodeOffline
	}
	return conn.Send(context.Background(), frame)
}
