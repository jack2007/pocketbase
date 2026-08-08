package agentapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/collections"
	centercrypto "github.com/pocketbase/pocketbase/apps/raypx2-center/internal/crypto"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
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
