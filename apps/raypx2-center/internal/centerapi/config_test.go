package centerapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/core"
)

type configHub struct {
	online   bool
	gets     []agenthub.ProxyResponse
	puts     []agenthub.ProxyRequest
	requests []agenthub.ProxyRequest
}

func (h *configHub) RequestProxy(_ context.Context, _ string, req agenthub.ProxyRequest) (agenthub.ProxyResponse, error) {
	h.requests = append(h.requests, req)
	if !h.online {
		return agenthub.ProxyResponse{}, agenthub.ErrNodeOffline
	}
	if req.Method == http.MethodGet {
		if len(h.gets) == 0 {
			return agenthub.ProxyResponse{}, errors.New("no get stub")
		}
		resp := h.gets[0]
		h.gets = h.gets[1:]
		return resp, nil
	}
	h.puts = append(h.puts, req)
	body, _ := json.Marshal(map[string]any{"ok": true})
	return agenthub.ProxyResponse{
		Status:  http.StatusOK,
		BodyB64: base64.StdEncoding.EncodeToString(body),
	}, nil
}

func TestPutNodeConfigOfflineStates(t *testing.T) {
	t.Run("database offline returns 409 without proxy", func(t *testing.T) {
		app, node, auth := newCenterTestApp(t)
		setConfigNode(t, app, node, "server", false)
		hub := &configHub{online: true}
		api := &API{hub: hub}

		response := performCenterRequest(t, app, auth, http.MethodPut,
			"/api/center/nodes/"+node.GetString("node_key")+"/config",
			node.GetString("node_key"), map[string]any{
				"content": map[string]any{"allow_targets": []any{"127.0.0.0/8"}},
			}, api.HandlePutNodeConfig)

		assertConfigError(t, response.Code, response.Body.Bytes(), http.StatusConflict, "node_offline")
		if len(hub.requests) != 0 {
			t.Fatalf("proxy requests = %d, want 0", len(hub.requests))
		}
	})

	t.Run("hub race offline returns 503", func(t *testing.T) {
		app, node, auth := newCenterTestApp(t)
		setConfigNode(t, app, node, "server", true)
		hub := &configHub{online: false}
		api := &API{hub: hub}

		response := performCenterRequest(t, app, auth, http.MethodPut,
			"/api/center/nodes/"+node.GetString("node_key")+"/config",
			node.GetString("node_key"), map[string]any{
				"content": map[string]any{"allow_targets": []any{"127.0.0.0/8"}},
			}, api.HandlePutNodeConfig)

		assertConfigError(t, response.Code, response.Body.Bytes(), http.StatusServiceUnavailable, "node_offline")
		if len(hub.requests) != 1 || hub.requests[0].Method != http.MethodGet {
			t.Fatalf("proxy requests = %#v", hub.requests)
		}
	})
}

func TestPutNodeConfigServerSendsTrimmedPatchAndAuditsSummary(t *testing.T) {
	app, node, auth := newCenterTestApp(t)
	setConfigNode(t, app, node, "server", true)
	before := map[string]any{
		"listen":        ":443",
		"allow_targets": []any{"10.0.0.0/8"},
		"startup":       map[string]any{"workers": float64(4)},
		"connection_config": map[string]any{
			"desired": map[string]any{
				"compression": map[string]any{"level": float64(3)},
			},
		},
	}
	after := map[string]any{
		"listen":        ":443",
		"allow_targets": []any{"127.0.0.0/8"},
		"connection_config": map[string]any{
			"desired": map[string]any{
				"compression": map[string]any{"level": float64(5)},
			},
		},
	}
	hub := &configHub{online: true, gets: []agenthub.ProxyResponse{configResponse(before), configResponse(after)}}
	api := &API{hub: hub}

	response := performCenterRequest(t, app, auth, http.MethodPut,
		"/api/center/nodes/"+node.GetString("node_key")+"/config",
		node.GetString("node_key"), map[string]any{
			"content": map[string]any{
				"allow_targets": []any{"127.0.0.0/8"},
				"listen":        ":8443",
				"connection": map[string]any{
					"compression":        map[string]any{"level": float64(5)},
					"max_send_rate_kbps": float64(100000),
				},
			},
		}, api.HandlePutNodeConfig)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(hub.puts) != 1 {
		t.Fatalf("writes = %d, want 1", len(hub.puts))
	}
	write := hub.puts[0]
	if write.Method != http.MethodPatch || write.Path != "/api/v1/server/config" {
		t.Fatalf("write request = %#v", write)
	}
	body := decodeConfigRequest(t, write)
	encodedBody, _ := json.Marshal(body)
	for _, forbidden := range []string{`"listen"`, `"max_send_rate_kbps"`, `"startup"`} {
		if bytes.Contains(encodedBody, []byte(forbidden)) {
			t.Fatalf("PATCH body contains %s: %s", forbidden, encodedBody)
		}
	}
	if _, ok := body["allow_targets"]; !ok {
		t.Fatalf("PATCH body = %#v", body)
	}

	var result struct {
		Applied       map[string]any `json:"applied"`
		IgnoredFields []string       `json:"ignored_fields"`
		RevisionID    string         `json:"revision_id"`
		AdminStatus   int            `json:"admin_status"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.RevisionID == "" || result.AdminStatus != http.StatusOK {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(strings.Join(result.IgnoredFields, ","), "listen") ||
		!strings.Contains(strings.Join(result.IgnoredFields, ","), "max_send_rate_kbps") {
		t.Fatalf("ignored_fields = %v", result.IgnoredFields)
	}
	appliedConnection := result.Applied["connection"].(map[string]any)
	if appliedConnection["compression"].(map[string]any)["level"] != float64(5) {
		t.Fatalf("applied = %#v", result.Applied)
	}

	revisions, err := app.FindRecordsByFilter(
		"config_revisions",
		"node = {:node} && kind = 'desired' && source = 'manual_edit'",
		"", 10, 0, map[string]any{"node": node.Id},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 1 {
		t.Fatalf("desired revisions = %d, want 1", len(revisions))
	}
	audits, err := app.FindRecordsByFilter(
		"audit_logs", "action = 'node.config.update' && node = {:node}",
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
	for _, key := range []string{"node_key", "role", "content_hash", "ignored_fields", "admin_status"} {
		if _, ok := summary[key]; !ok {
			t.Fatalf("audit summary missing %q: %s", key, summaryJSON)
		}
	}
	if len(summary) != 5 || bytes.Contains(summaryJSON, []byte(`"content"`)) ||
		bytes.Contains(summaryJSON, []byte("127.0.0.0/8")) {
		t.Fatalf("unsafe audit summary: %s", summaryJSON)
	}
}

func TestPutNodeConfigAuditsSuccessfulWriteWhenAppliedGetFails(t *testing.T) {
	app, node, auth := newCenterTestApp(t)
	setConfigNode(t, app, node, "server", true)
	hub := &configHub{online: true, gets: []agenthub.ProxyResponse{configResponse(map[string]any{
		"allow_targets": []any{"10.0.0.0/8"},
	})}}
	api := &API{hub: hub}

	response := performCenterRequest(t, app, auth, http.MethodPut,
		"/api/center/nodes/"+node.GetString("node_key")+"/config",
		node.GetString("node_key"), map[string]any{
			"content": map[string]any{"allow_targets": []any{"127.0.0.0/8"}},
		}, api.HandlePutNodeConfig)

	assertConfigError(t, response.Code, response.Body.Bytes(), http.StatusBadGateway, "admin_unreachable")
	revisions, err := app.FindRecordsByFilter(
		"config_revisions",
		"node = {:node} && kind = 'desired' && source = 'manual_edit'",
		"", 10, 0, map[string]any{"node": node.Id},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 1 {
		t.Fatalf("desired revisions = %d, want 1", len(revisions))
	}
	audits, err := app.FindRecordsByFilter(
		"audit_logs", "action = 'node.config.update' && node = {:node}",
		"", 10, 0, map[string]any{"node": node.Id},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) != 1 {
		t.Fatalf("audits = %d, want 1 after successful write", len(audits))
	}
}

func TestPutNodeConfigClientNormalizesAdminPeersBeforeMerge(t *testing.T) {
	app, node, auth := newCenterTestApp(t)
	setConfigNode(t, app, node, "client", true)
	actual := map[string]any{
		"version": float64(1),
		"peers": []any{
			map[string]any{
				"id":         "peer-a",
				"proto_peer": "old:443",
				"status":     "connected",
				"connection": map[string]any{
					"encryption":         "enabled",
					"max_send_rate_kbps": float64(0),
					"runtime_state":      "ready",
				},
			},
			map[string]any{"id": "peer-b", "proto_peer": "keep:443"},
		},
	}
	hub := &configHub{online: true, gets: []agenthub.ProxyResponse{configResponse(actual), configResponse(actual)}}
	api := &API{hub: hub}

	response := performCenterRequest(t, app, auth, http.MethodPut,
		"/api/center/nodes/"+node.GetString("node_key")+"/config",
		node.GetString("node_key"), map[string]any{
			"content": map[string]any{"peers": []any{map[string]any{
				"peer_id": "peer-a",
				"connection": map[string]any{
					"min_send_rate_kbps": float64(1000),
					"max_send_rate_kbps": float64(2000),
				},
			}}},
		}, api.HandlePutNodeConfig)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"ignored_fields":[]`)) {
		t.Fatalf("ignored_fields must be an array: %s", response.Body.String())
	}
	if len(hub.puts) != 1 || hub.puts[0].Method != http.MethodPut ||
		hub.puts[0].Path != "/api/v1/config" {
		t.Fatalf("writes = %#v", hub.puts)
	}
	body := decodeConfigRequest(t, hub.puts[0])
	peers := body["peers"].([]any)
	if len(peers) != 2 ||
		peers[0].(map[string]any)["peer_id"] != "peer-a" ||
		peers[1].(map[string]any)["peer_id"] != "peer-b" ||
		peers[1].(map[string]any)["quic_peer"] != "keep:443" {
		t.Fatalf("merged peers = %#v", peers)
	}
	connection := peers[0].(map[string]any)["connection"].(map[string]any)
	if connection["encryption"] != "enabled" ||
		connection["max_send_rate_kbps"] != float64(2000) {
		t.Fatalf("peer-a connection = %#v", connection)
	}
	var result struct {
		Applied map[string]any `json:"applied"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	appliedPeer := result.Applied["peers"].([]any)[0].(map[string]any)
	if _, ok := appliedPeer["status"]; ok {
		t.Fatalf("applied contains non-writable status: %#v", appliedPeer)
	}
	if _, ok := appliedPeer["connection"].(map[string]any)["runtime_state"]; ok {
		t.Fatalf("applied contains non-writable runtime_state: %#v", appliedPeer)
	}
}

func TestPutNodeConfigRejectsSecretsAndEmptyUpdatesLocally(t *testing.T) {
	tests := []struct {
		name    string
		role    string
		content map[string]any
		code    string
	}{
		{
			name: "tls key", role: "server", code: "secret_field_forbidden",
			content: map[string]any{
				"allow_targets": []any{"127.0.0.0/8"},
				"tls":           map[string]any{"key": "SECRET"},
			},
		},
		{
			name: "enroll secret", role: "client", code: "secret_field_forbidden",
			content: map[string]any{"peers": []any{map[string]any{
				"peer_id": "peer-a", "enroll_secret": "SECRET",
			}}},
		},
		{
			name: "trimmed empty", role: "server", code: "empty_config_update",
			content: map[string]any{"listen": ":443"},
		},
		{
			name: "empty peers", role: "client", code: "empty_config_update",
			content: map[string]any{"peers": []any{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, node, auth := newCenterTestApp(t)
			setConfigNode(t, app, node, tt.role, true)
			hub := &configHub{online: true}
			api := &API{hub: hub}

			response := performCenterRequest(t, app, auth, http.MethodPut,
				"/api/center/nodes/"+node.GetString("node_key")+"/config",
				node.GetString("node_key"), map[string]any{"content": tt.content},
				api.HandlePutNodeConfig)

			assertConfigError(t, response.Code, response.Body.Bytes(), http.StatusBadRequest, tt.code)
			if len(hub.requests) != 0 {
				t.Fatalf("proxy requests = %d, want 0", len(hub.requests))
			}
		})
	}
}

func TestGetNodeConfigOnlineRedactsAndProjects(t *testing.T) {
	app, node, auth := newCenterTestApp(t)
	setConfigNode(t, app, node, "server", true)
	hub := &configHub{online: true, gets: []agenthub.ProxyResponse{configResponse(map[string]any{
		"allow_targets": []any{"10.0.0.0/8"},
		"admin_token":   "SECRET",
		"tls":           map[string]any{"key": "SECRET", "ca": "ca.pem"},
	})}}
	api := &API{hub: hub}

	response := performCenterRequest(t, app, auth, http.MethodGet,
		"/api/center/nodes/"+node.GetString("node_key")+"/config",
		node.GetString("node_key"), nil, api.HandleGetNodeConfig)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result struct {
		Live          map[string]any `json:"live"`
		EditorDraft   map[string]any `json:"editor_draft"`
		WritablePaths []string       `json:"writable_paths"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Live["admin_token"] != "[REDACTED]" ||
		result.Live["tls"].(map[string]any)["key"] != "[REDACTED]" {
		t.Fatalf("live = %#v", result.Live)
	}
	if result.EditorDraft["allow_targets"] == nil || len(result.WritablePaths) == 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestGetNodeConfigClientEditorDraftProjectsWritableFields(t *testing.T) {
	app, node, auth := newCenterTestApp(t)
	setConfigNode(t, app, node, "client", true)
	hub := &configHub{online: true, gets: []agenthub.ProxyResponse{configResponse(map[string]any{
		"peers": []any{map[string]any{
			"id": "peer-a", "proto_peer": "host:443", "status": "connected",
		}},
	})}}
	api := &API{hub: hub}

	response := performCenterRequest(t, app, auth, http.MethodGet,
		"/api/center/nodes/"+node.GetString("node_key")+"/config",
		node.GetString("node_key"), nil, api.HandleGetNodeConfig)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result struct {
		Live        map[string]any `json:"live"`
		EditorDraft map[string]any `json:"editor_draft"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	livePeer := result.Live["peers"].([]any)[0].(map[string]any)
	draftPeer := result.EditorDraft["peers"].([]any)[0].(map[string]any)
	if livePeer["status"] != "connected" {
		t.Fatalf("live must retain status: %#v", livePeer)
	}
	if draftPeer["peer_id"] != "peer-a" || draftPeer["quic_peer"] != "host:443" {
		t.Fatalf("editor_draft aliases not normalized: %#v", draftPeer)
	}
	if _, ok := draftPeer["status"]; ok {
		t.Fatalf("editor_draft contains non-writable status: %#v", draftPeer)
	}
}

func TestGetNodeConfigOfflineReturnsEmptyDraft(t *testing.T) {
	app, node, auth := newCenterTestApp(t)
	setConfigNode(t, app, node, "client", false)
	hub := &configHub{online: true}
	api := &API{hub: hub}

	response := performCenterRequest(t, app, auth, http.MethodGet,
		"/api/center/nodes/"+node.GetString("node_key")+"/config",
		node.GetString("node_key"), nil, api.HandleGetNodeConfig)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result struct {
		Live        any            `json:"live"`
		EditorDraft map[string]any `json:"editor_draft"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Live != nil || len(result.EditorDraft) != 0 {
		t.Fatalf("result = %#v", result)
	}
	if len(hub.requests) != 0 {
		t.Fatalf("proxy requests = %d, want 0", len(hub.requests))
	}
}

func TestNodeConfigUnknownRoleReadsButCannotWrite(t *testing.T) {
	app, node, auth := newCenterTestApp(t)
	setConfigNode(t, app, node, "unknown", true)
	hub := &configHub{online: true, gets: []agenthub.ProxyResponse{configResponse(map[string]any{
		"status": "available", "admin_token": "SECRET",
	})}}
	api := &API{hub: hub}

	getResponse := performCenterRequest(t, app, auth, http.MethodGet,
		"/api/center/nodes/"+node.GetString("node_key")+"/config",
		node.GetString("node_key"), nil, api.HandleGetNodeConfig)
	if getResponse.Code != http.StatusOK ||
		!bytes.Contains(getResponse.Body.Bytes(), []byte(`"live"`)) ||
		!bytes.Contains(getResponse.Body.Bytes(), []byte(`[REDACTED]`)) {
		t.Fatalf("GET response = %d %s", getResponse.Code, getResponse.Body.String())
	}

	putResponse := performCenterRequest(t, app, auth, http.MethodPut,
		"/api/center/nodes/"+node.GetString("node_key")+"/config",
		node.GetString("node_key"), map[string]any{
			"content": map[string]any{"allow_targets": []any{"127.0.0.0/8"}},
		}, api.HandlePutNodeConfig)
	assertConfigError(t, putResponse.Code, putResponse.Body.Bytes(), http.StatusBadRequest, "unsupported_node_role")
	if len(hub.requests) != 1 {
		t.Fatalf("PUT must not call proxy, requests = %#v", hub.requests)
	}
}

func setConfigNode(t *testing.T, app core.App, node *core.Record, role string, online bool) {
	t.Helper()
	node.Set("role", role)
	node.Set("online", online)
	if err := app.Save(node); err != nil {
		t.Fatal(err)
	}
}

func configResponse(content map[string]any) agenthub.ProxyResponse {
	body, _ := json.Marshal(content)
	return agenthub.ProxyResponse{
		Status:  http.StatusOK,
		BodyB64: base64.StdEncoding.EncodeToString(body),
	}
}

func decodeConfigRequest(t *testing.T, request agenthub.ProxyRequest) map[string]any {
	t.Helper()
	body, err := base64.StdEncoding.DecodeString(request.BodyB64)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func assertConfigError(t *testing.T, gotStatus int, body []byte, wantStatus int, code string) {
	t.Helper()
	if gotStatus != wantStatus || !bytes.Contains(body, []byte(code)) {
		t.Fatalf("response = %d %s, want %d %s", gotStatus, body, wantStatus, code)
	}
}
