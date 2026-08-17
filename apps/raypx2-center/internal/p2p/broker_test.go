package p2p

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/protocol"
)

type rawConn struct {
	id        string
	sessionID string
	mu        sync.Mutex
	raw       [][]byte
}

func (c *rawConn) ID() string        { return c.id }
func (c *rawConn) SessionID() string { return c.sessionID }
func (c *rawConn) Send(context.Context, protocol.Frame) error {
	return nil
}
func (c *rawConn) Close(string) error { return nil }
func (c *rawConn) SendRaw(_ context.Context, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.raw = append(c.raw, append([]byte(nil), payload...))
	return nil
}
func (c *rawConn) last() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.raw) == 0 {
		return nil
	}
	return c.raw[len(c.raw)-1]
}

func TestBrokerCreateSendsInviteToBoth(t *testing.T) {
	hub := agenthub.New()
	client := &rawConn{id: "c", sessionID: "cs"}
	server := &rawConn{id: "s", sessionID: "ss"}
	hub.Register("client-key", client)
	hub.Register("server-key", server)
	broker := NewBroker(hub, Config{
		STUNURLs:         []string{DefaultSTUNURL},
		TURNURLs:         []string{DefaultTURNURL},
		SharedSecretFile: "/no/such/secret",
		CredentialTTL:    3600,
	})
	session, err := broker.Create(CreateRequest{
		ClientNodeKey: "client-key",
		ServerNodeKey: "server-key",
		GrantMACKey:   VerifyKeyFromEnrollSecret("server-enroll"),
		Now:           time.Unix(1_700_000_000, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.Epoch != 1 || session.ConnectionID != "conn-0" {
		t.Fatalf("session = %#v", session)
	}
	var frame map[string]any
	if err := json.Unmarshal(client.last(), &frame); err != nil {
		t.Fatal(err)
	}
	if frame["type"] != "invite" {
		t.Fatalf("type = %v", frame["type"])
	}
	payload, _ := frame["payload"].(map[string]any)
	if payload["grant"] == "" {
		t.Fatal("invite missing grant")
	}
	if _, ok := payload["turn"]; ok {
		t.Fatal("turn must be omitted when secret is missing")
	}
	if err := broker.Forward("client-key", []byte(
		`{"id":"x","session_id":"`+session.SessionID+`","connection_id":"conn-0","epoch":1,"seq":2,"type":"candidate","payload":{}}`,
	)); err != nil {
		t.Fatal(err)
	}
	if len(server.raw) != 2 {
		t.Fatalf("server frames = %d", len(server.raw))
	}
}
