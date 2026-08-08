package agentapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/collections"
	centercrypto "github.com/pocketbase/pocketbase/apps/raypx2-center/internal/crypto"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/protocol"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/pocketbase/pocketbase/tools/types"
)

func TestApplyStatusSummaryUpdatesNodeAndUpsertsStatus(t *testing.T) {
	app, node := newWSTestApp(t)
	payload := json.RawMessage(`{
		"health_status":"healthy",
		"uptime_seconds":42,
		"last_error":"",
		"config_hash":"sha256:abc",
		"connections":3
	}`)

	if err := applyStatusSummary(app, node, payload); err != nil {
		t.Fatal(err)
	}
	if err := applyStatusSummary(app, node, payload); err != nil {
		t.Fatal(err)
	}

	updated, err := app.FindRecordById("nodes", node.Id)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.GetBool("online") || updated.GetDateTime("last_seen_at").IsZero() {
		t.Fatalf("node not marked online/seen: %#v", updated)
	}
	statuses, err := app.FindRecordsByFilter("node_status", "node = {:node}", "", 10, 0, map[string]any{"node": node.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 {
		t.Fatalf("status rows = %d, want 1", len(statuses))
	}
	if statuses[0].GetString("health_status") != "healthy" ||
		statuses[0].GetInt("uptime_seconds") != 42 ||
		statuses[0].GetString("config_hash") != "sha256:abc" {
		t.Fatalf("unexpected status: %#v", statuses[0])
	}
}

func TestApplyConfigSnapshotCreatesActualRevision(t *testing.T) {
	app, node := newWSTestApp(t)
	payload := json.RawMessage(`{
		"content_hash":"sha256:def",
		"content":{"server":{"listen":"127.0.0.1:2345"}}
	}`)

	if err := applyConfigSnapshot(app, node, payload); err != nil {
		t.Fatal(err)
	}

	revisions, err := app.FindRecordsByFilter("config_revisions", "node = {:node}", "", 10, 0, map[string]any{"node": node.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 1 {
		t.Fatalf("revision rows = %d, want 1", len(revisions))
	}
	revision := revisions[0]
	if revision.GetString("kind") != "actual" ||
		revision.GetString("source") != "pull" ||
		revision.GetString("content_hash") != "sha256:def" {
		t.Fatalf("unexpected revision: %#v", revision)
	}
}

func TestLookupSessionReturnsLiveSessionAndNode(t *testing.T) {
	app, node := newWSTestApp(t)
	token := "session-token"
	hash, err := centercrypto.HashToken(token)
	if err != nil {
		t.Fatal(err)
	}
	collection, err := app.FindCollectionByNameOrId("agent_sessions")
	if err != nil {
		t.Fatal(err)
	}
	session := core.NewRecord(collection)
	session.Set("node", node.Id)
	session.Set("token_hash", hash)
	session.Set("expires_at", types.NowDateTime().Add(time.Minute))
	if err := app.Save(session); err != nil {
		t.Fatal(err)
	}

	gotSession, gotNode, err := LookupSession(app, "Bearer "+token)
	if err != nil {
		t.Fatal(err)
	}
	if gotSession.Id != session.Id || gotNode.Id != node.Id {
		t.Fatalf("lookup returned session %q node %q", gotSession.Id, gotNode.Id)
	}
}

func TestHandleWSWelcomeAndPingPong(t *testing.T) {
	app, node := newWSTestApp(t)
	testHub := agenthub.New()
	SetHub(testHub)
	t.Cleanup(func() { SetHub(agenthub.New()) })

	token := createWSTestSession(t, app, node)
	ts := newWSIntegrationServer(t, app)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/agent/ws"
	conn, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + token},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.Close(websocket.StatusNormalClosure, "test done")
	})
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
	}

	welcomeCtx, welcomeCancel := context.WithTimeout(ctx, 2*time.Second)
	defer welcomeCancel()
	var welcome protocol.Frame
	if err := wsjson.Read(welcomeCtx, conn, &welcome); err != nil {
		t.Fatalf("read welcome: %v", err)
	}
	if welcome.Type != "welcome" {
		t.Fatalf("frame type = %q, want welcome", welcome.Type)
	}

	nodeKey := node.GetString("node_key")
	waitForWSReady(t, app, node.Id, nodeKey, testHub, 2*time.Second)

	pingID := "integration-ping"
	if err := wsjson.Write(ctx, conn, protocol.Frame{
		Type:    "ping",
		ID:      pingID,
		TS:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	pongCtx, pongCancel := context.WithTimeout(ctx, 2*time.Second)
	defer pongCancel()
	var pong protocol.Frame
	if err := wsjson.Read(pongCtx, conn, &pong); err != nil {
		t.Fatalf("read pong: %v", err)
	}
	if pong.Type != "pong" || pong.ID != pingID {
		t.Fatalf("pong = %#v, want type pong id %q", pong, pingID)
	}
}

func waitForWSReady(t *testing.T, app core.App, nodeID, nodeKey string, hub *agenthub.Hub, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !hub.HasConnection(nodeKey) {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		updated, err := app.FindRecordById("nodes", nodeID)
		if err == nil && updated.GetBool("online") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("node should be registered in hub and marked online after welcome")
}

func createWSTestSession(t *testing.T, app core.App, node *core.Record) string {
	t.Helper()
	token := "ws-test-token"
	hash, err := centercrypto.HashToken(token)
	if err != nil {
		t.Fatal(err)
	}
	collection, err := app.FindCollectionByNameOrId("agent_sessions")
	if err != nil {
		t.Fatal(err)
	}
	session := core.NewRecord(collection)
	session.Set("node", node.Id)
	session.Set("token_hash", hash)
	session.Set("expires_at", types.NowDateTime().Add(time.Minute))
	if err := app.Save(session); err != nil {
		t.Fatal(err)
	}
	return token
}

func newWSIntegrationServer(t *testing.T, app core.App) *httptest.Server {
	t.Helper()
	pbRouter := router.NewRouter(func(w http.ResponseWriter, r *http.Request) (*core.RequestEvent, router.EventCleanupFunc) {
		event := new(core.RequestEvent)
		event.Response = w
		event.Request = r
		event.App = app
		return event, nil
	})
	pbRouter.GET("/api/agent/ws", HandleWS)
	mux, err := pbRouter.BuildMux()
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(mux)
}

func newWSTestApp(t *testing.T) (*tests.TestApp, *core.Record) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	if err := collections.EnsureCollections(app); err != nil {
		t.Fatal(err)
	}
	_, hash, err := centercrypto.GenerateEnrollSecret()
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := app.FindCollectionByNameOrId("nodes")
	if err != nil {
		t.Fatal(err)
	}
	node := core.NewRecord(nodes)
	node.Set("node_key", "node-ws")
	node.Set("enroll_secret_hash", hash)
	node.Set("enroll_status", "active")
	node.Set("role", "unknown")
	if err := app.Save(node); err != nil {
		t.Fatal(err)
	}
	return app, node
}
