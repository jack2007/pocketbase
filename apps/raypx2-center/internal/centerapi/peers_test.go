package centerapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/core"
)

func TestDeleteNodePeerSuccessWritesRevisionAndAudit(t *testing.T) {
	app, node, auth := newCenterTestApp(t)
	setConfigNode(t, app, node, "client", true)
	actual := map[string]any{
		"version": float64(1),
		"peers": []any{
			map[string]any{"peer_id": "peer-a", "quic_peer": "a:443"},
			map[string]any{"peer_id": "peer-b", "quic_peer": "b:443"},
		},
	}
	hub := &configHub{online: true, gets: []agenthub.ProxyResponse{configResponse(actual)}}
	api := &API{hub: hub}

	response := performPeerDeleteRequest(t, app, auth, node.GetString("node_key"), "peer-a", api)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["peer_id"] != "peer-a" || result["revision_id"] == "" || result["admin_status"] != float64(200) {
		t.Fatalf("result = %#v", result)
	}
	if len(hub.puts) != 1 || hub.puts[0].Method != http.MethodPut || hub.puts[0].Path != "/api/v1/config" {
		t.Fatalf("writes = %#v", hub.puts)
	}
	body := decodeConfigRequest(t, hub.puts[0])
	peers := body["peers"].([]any)
	if len(peers) != 1 || peers[0].(map[string]any)["peer_id"] != "peer-b" {
		t.Fatalf("PUT peers = %#v", peers)
	}

	revisions, err := app.FindRecordsByFilter(
		"config_revisions",
		"node = {:node} && kind = 'desired' && source = 'peer_delete'",
		"", 10, 0, map[string]any{"node": node.Id},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 1 {
		t.Fatalf("desired revisions = %d, want 1", len(revisions))
	}
	audits, err := app.FindRecordsByFilter(
		"audit_logs", "action = 'node.peer.delete' && node = {:node}",
		"", 10, 0, map[string]any{"node": node.Id},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) != 1 {
		t.Fatalf("audits = %d, want 1", len(audits))
	}
	summaryJSON, _ := json.Marshal(audits[0].Get("request_summary"))
	var summary map[string]any
	if err := json.Unmarshal(summaryJSON, &summary); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"node_key", "peer_id", "content_hash", "admin_status"} {
		if _, ok := summary[key]; !ok {
			t.Fatalf("audit summary missing %q: %s", key, summaryJSON)
		}
	}
	if len(summary) != 4 || bytes.Contains(summaryJSON, []byte(`"peers"`)) {
		t.Fatalf("unsafe audit summary: %s", summaryJSON)
	}
}

func TestDeleteNodePeerNotFound(t *testing.T) {
	app, node, auth := newCenterTestApp(t)
	setConfigNode(t, app, node, "client", true)
	actual := map[string]any{
		"peers": []any{map[string]any{"peer_id": "peer-a"}},
	}
	hub := &configHub{online: true, gets: []agenthub.ProxyResponse{configResponse(actual)}}
	api := &API{hub: hub}

	response := performPeerDeleteRequest(t, app, auth, node.GetString("node_key"), "missing", api)

	assertConfigError(t, response.Code, response.Body.Bytes(), http.StatusNotFound, "peer_not_found")
	if len(hub.puts) != 0 {
		t.Fatalf("must not PUT when peer missing: %#v", hub.puts)
	}
}

func TestDeleteNodePeerRejectsNonClientAndOffline(t *testing.T) {
	t.Run("server role", func(t *testing.T) {
		app, node, auth := newCenterTestApp(t)
		setConfigNode(t, app, node, "server", true)
		hub := &configHub{online: true}
		api := &API{hub: hub}

		response := performPeerDeleteRequest(t, app, auth, node.GetString("node_key"), "peer-a", api)

		assertConfigError(t, response.Code, response.Body.Bytes(), http.StatusBadRequest, "unsupported_node_role")
		if len(hub.requests) != 0 {
			t.Fatalf("proxy requests = %#v", hub.requests)
		}
	})

	t.Run("database offline", func(t *testing.T) {
		app, node, auth := newCenterTestApp(t)
		setConfigNode(t, app, node, "client", false)
		hub := &configHub{online: true}
		api := &API{hub: hub}

		response := performPeerDeleteRequest(t, app, auth, node.GetString("node_key"), "peer-a", api)

		assertConfigError(t, response.Code, response.Body.Bytes(), http.StatusConflict, "node_offline")
		if len(hub.requests) != 0 {
			t.Fatalf("proxy requests = %#v", hub.requests)
		}
	})

	t.Run("hub race offline", func(t *testing.T) {
		app, node, auth := newCenterTestApp(t)
		setConfigNode(t, app, node, "client", true)
		hub := &configHub{online: false}
		api := &API{hub: hub}

		response := performPeerDeleteRequest(t, app, auth, node.GetString("node_key"), "peer-a", api)

		assertConfigError(t, response.Code, response.Body.Bytes(), http.StatusServiceUnavailable, "node_offline")
	})
}

func performPeerDeleteRequest(
	t *testing.T,
	app core.App,
	auth *core.Record,
	nodeKey, peerID string,
	api *API,
) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/center/nodes/" + nodeKey + "/peers/" + peerID
	request := httptest.NewRequest(http.MethodDelete, path, nil)
	request.SetPathValue("node_key", nodeKey)
	request.SetPathValue("peer_id", peerID)
	response := httptest.NewRecorder()
	event := &core.RequestEvent{App: app, Auth: auth}
	event.Request = request
	event.Response = response
	if err := api.HandleDeleteNodePeer(event); err != nil {
		t.Fatal(err)
	}
	return response
}
