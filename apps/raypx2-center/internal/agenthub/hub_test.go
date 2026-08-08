package agenthub_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/protocol"
)

type fakeConn struct {
	id        string
	sessionID string

	mu     sync.Mutex
	sent   []protocol.Frame
	closed bool
}

func (c *fakeConn) ID() string        { return c.id }
func (c *fakeConn) SessionID() string { return c.sessionID }

func (c *fakeConn) Send(_ context.Context, frame protocol.Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, frame)
	return nil
}

func (c *fakeConn) Close(string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *fakeConn) snapshot() ([]protocol.Frame, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]protocol.Frame(nil), c.sent...), c.closed
}

func TestHubReplaceConnectionSendsByeClosesAndRevokesOldSession(t *testing.T) {
	var revoked string
	h := agenthub.New(agenthub.WithSessionRevoker(func(sessionID string) error {
		revoked = sessionID
		return nil
	}))
	a := &fakeConn{id: "a", sessionID: "session-a"}
	b := &fakeConn{id: "b", sessionID: "session-b"}

	if replaced := h.Register("n1", a); replaced {
		t.Fatal("first registration unexpectedly replaced a connection")
	}
	if replaced := h.Register("n1", b); !replaced {
		t.Fatal("second registration did not report replacement")
	}

	sent, closed := a.snapshot()
	if !closed {
		t.Fatal("expected old connection closed")
	}
	if len(sent) != 1 || sent[0].Type != "bye" {
		t.Fatalf("old connection frames = %#v, want one bye", sent)
	}
	if revoked != "session-a" {
		t.Fatalf("revoked session = %q, want session-a", revoked)
	}
}

func TestHubRequestProxyCorrelatesResponse(t *testing.T) {
	h := agenthub.New()
	conn := &fakeConn{id: "a", sessionID: "session-a"}
	h.Register("n1", conn)

	result := make(chan agenthub.ProxyResponse, 1)
	errs := make(chan error, 1)
	go func() {
		response, err := h.RequestProxy(context.Background(), "n1", agenthub.ProxyRequest{
			Method: "GET",
			Path:   "/api/v1/status",
		})
		if err != nil {
			errs <- err
			return
		}
		result <- response
	}()

	var requestFrame protocol.Frame
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		sent, _ := conn.snapshot()
		if len(sent) > 0 {
			requestFrame = sent[0]
			break
		}
		time.Sleep(time.Millisecond)
	}
	if requestFrame.Type != "http_proxy_req" || requestFrame.ID == "" {
		t.Fatalf("request frame = %#v", requestFrame)
	}

	payload, err := json.Marshal(agenthub.ProxyResponse{Status: 200, BodyB64: "b2s="})
	if err != nil {
		t.Fatal(err)
	}
	if handled := h.HandleFrame("n1", protocol.Frame{
		Type:    "http_proxy_res",
		ID:      requestFrame.ID,
		Payload: payload,
	}); !handled {
		t.Fatal("proxy response was not handled")
	}

	select {
	case err := <-errs:
		t.Fatal(err)
	case response := <-result:
		if response.Status != 200 || response.BodyB64 != "b2s=" {
			t.Fatalf("response = %#v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for correlated response")
	}
}

func TestHubRequestProxyLimitsInflightPerNode(t *testing.T) {
	h := agenthub.New()
	conn := &fakeConn{id: "a", sessionID: "session-a"}
	h.Register("n1", conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for range 8 {
		go func() {
			_, _ = h.RequestProxy(ctx, "n1", agenthub.ProxyRequest{Method: "GET", Path: "/api/v1/status"})
		}()
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		sent, _ := conn.snapshot()
		if len(sent) == 8 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	_, err := h.RequestProxy(context.Background(), "n1", agenthub.ProxyRequest{
		Method: "GET",
		Path:   "/api/v1/status",
	})
	if !errors.Is(err, agenthub.ErrProxyInflightLimit) {
		t.Fatalf("error = %v, want ErrProxyInflightLimit", err)
	}
}
