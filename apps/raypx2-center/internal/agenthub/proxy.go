package agenthub

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/protocol"
)

var (
	ErrNodeOffline        = errors.New("node offline")
	ErrProxyInflightLimit = errors.New("proxy inflight limit reached")
)

const maxProxyInflightPerNode = 8

type ProxyRequest struct {
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Headers   map[string]string `json:"headers,omitempty"`
	BodyB64   string            `json:"body_b64,omitempty"`
	TimeoutMS int               `json:"timeout_ms,omitempty"`
}

type ProxyResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	BodyB64 string            `json:"body_b64,omitempty"`
	Error   string            `json:"error,omitempty"`
}

type pendingResponse struct {
	nodeKey string
	result  chan proxyResult
}

type proxyResult struct {
	response ProxyResponse
	err      error
}

func (h *Hub) RequestProxy(ctx context.Context, nodeKey string, request ProxyRequest) (ProxyResponse, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return ProxyResponse{}, err
	}
	id := uuid.NewString()
	result := make(chan proxyResult, 1)

	h.mu.Lock()
	conn := h.connections[nodeKey]
	if conn == nil {
		h.mu.Unlock()
		return ProxyResponse{}, ErrNodeOffline
	}
	inflight := 0
	for _, pending := range h.pending {
		if pending.nodeKey == nodeKey {
			inflight++
		}
	}
	if inflight >= maxProxyInflightPerNode {
		h.mu.Unlock()
		return ProxyResponse{}, ErrProxyInflightLimit
	}
	h.pending[id] = pendingResponse{nodeKey: nodeKey, result: result}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.pending, id)
		h.mu.Unlock()
	}()
	if err := conn.Send(ctx, protocol.Frame{
		Type:    "http_proxy_req",
		ID:      id,
		TS:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload: payload,
	}); err != nil {
		return ProxyResponse{}, err
	}

	select {
	case <-ctx.Done():
		return ProxyResponse{}, ctx.Err()
	case response := <-result:
		return response.response, response.err
	}
}

func (h *Hub) HandleFrame(nodeKey string, frame protocol.Frame) bool {
	if frame.Type != "http_proxy_res" || frame.ID == "" {
		return false
	}
	h.mu.Lock()
	pending, ok := h.pending[frame.ID]
	if ok && pending.nodeKey == nodeKey {
		delete(h.pending, frame.ID)
	}
	h.mu.Unlock()
	if !ok || pending.nodeKey != nodeKey {
		return false
	}

	var response ProxyResponse
	if err := json.Unmarshal(frame.Payload, &response); err != nil {
		pending.result <- proxyResult{err: err}
		return true
	}
	pending.result <- proxyResult{response: response}
	return true
}
