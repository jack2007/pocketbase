package agentapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agentapi"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/collections"
	centercrypto "github.com/pocketbase/pocketbase/apps/raypx2-center/internal/crypto"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
)

func TestHandleEnrollCreatesSessionUpdatesNodeAndAudits(t *testing.T) {
	app, node, secret := newEnrollTestApp(t)

	response := performJSONRequest(t, app, agentapi.HandleEnroll, nil, map[string]any{
		"node_key":      node.GetString("node_key"),
		"enroll_secret": secret,
		"hostname":      "edge-01",
		"version":       "1.2.3",
		"role":          "server",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}

	var body struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Token == "" || body.ExpiresAt == "" {
		t.Fatalf("missing session response fields: %s", response.Body)
	}

	sessions, err := app.FindRecordsByFilter("agent_sessions", "node = {:node}", "", 10, 0, mapParams("node", node.Id))
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %d, err = %v", len(sessions), err)
	}
	if !centercrypto.VerifySecret(sessions[0].GetString("token_hash"), body.Token) {
		t.Fatal("stored token hash does not match returned token")
	}
	ttl := sessions[0].GetDateTime("expires_at").Time().Sub(time.Now())
	if ttl < 29*time.Minute || ttl > 31*time.Minute {
		t.Fatalf("session TTL = %v", ttl)
	}

	updatedNode, err := app.FindRecordById("nodes", node.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := updatedNode.GetString("hostname"); got != "edge-01" {
		t.Fatalf("hostname = %q", got)
	}
	if got := updatedNode.GetString("agent_version"); got != "1.2.3" {
		t.Fatalf("agent_version = %q", got)
	}
	if got := updatedNode.GetString("role"); got != "server" {
		t.Fatalf("role = %q", got)
	}
	assertAuditCount(t, app, node.Id, 1)
}

func TestHandleEnrollReturnsGenericErrorAndAuditsFailure(t *testing.T) {
	app, node, _ := newEnrollTestApp(t)

	response := performJSONRequest(t, app, agentapi.HandleEnroll, nil, map[string]any{
		"node_key":      node.GetString("node_key"),
		"enroll_secret": "wrong",
	})
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	if got := response.Body.String(); got != "{\"message\":\"invalid credentials\"}\n" {
		t.Fatalf("body = %q", got)
	}
	assertAuditCount(t, app, node.Id, 1)
}

func TestHandleEnrollHidesValidationFailureAndAudits(t *testing.T) {
	app, node, secret := newEnrollTestApp(t)

	response := performJSONRequest(t, app, agentapi.HandleEnroll, nil, map[string]any{
		"node_key":      node.GetString("node_key"),
		"enroll_secret": secret,
		"role":          "invalid-role",
	})
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	if got := response.Body.String(); got != "{\"message\":\"invalid credentials\"}\n" {
		t.Fatalf("body = %q", got)
	}
	assertAuditCount(t, app, node.Id, 1)
}

func TestHandleRefreshRejectsInvalidBearer(t *testing.T) {
	app, _, _ := newEnrollTestApp(t)

	cases := []struct {
		name    string
		headers map[string]string
	}{
		{"missing authorization", nil},
		{"malformed authorization", map[string]string{"Authorization": "not-bearer"}},
		{"wrong token", map[string]string{"Authorization": "Bearer wrong-token"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := performJSONRequest(t, app, agentapi.HandleRefresh, tc.headers, nil)
			assertInvalidCredentials(t, response)
		})
	}
}

func TestHandleRefreshRejectsExpiredSession(t *testing.T) {
	app, node, secret := newEnrollTestApp(t)
	token := enrollAndGetToken(t, app, node, secret)

	sessions, err := app.FindRecordsByFilter("agent_sessions", "node = {:node}", "", 10, 0, mapParams("node", node.Id))
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %d, err = %v", len(sessions), err)
	}
	sessions[0].Set("expires_at", types.NowDateTime().Add(-time.Hour))
	if err := app.Save(sessions[0]); err != nil {
		t.Fatal(err)
	}

	response := performJSONRequest(t, app, agentapi.HandleRefresh, map[string]string{
		"Authorization": "Bearer " + token,
	}, nil)
	assertInvalidCredentials(t, response)
}

func TestHandleRefreshRejectsRevokedSession(t *testing.T) {
	app, node, secret := newEnrollTestApp(t)
	token := enrollAndGetToken(t, app, node, secret)

	sessions, err := app.FindRecordsByFilter("agent_sessions", "node = {:node}", "", 10, 0, mapParams("node", node.Id))
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %d, err = %v", len(sessions), err)
	}
	sessions[0].Set("revoked_at", types.NowDateTime())
	if err := app.Save(sessions[0]); err != nil {
		t.Fatal(err)
	}

	response := performJSONRequest(t, app, agentapi.HandleRefresh, map[string]string{
		"Authorization": "Bearer " + token,
	}, nil)
	assertInvalidCredentials(t, response)
}

func TestHandleRefreshRotatesValidSessionToken(t *testing.T) {
	app, node, secret := newEnrollTestApp(t)
	enrolledToken := enrollAndGetToken(t, app, node, secret)

	refreshResponse := performJSONRequest(t, app, agentapi.HandleRefresh, map[string]string{
		"Authorization": "Bearer " + enrolledToken,
	}, nil)
	if refreshResponse.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", refreshResponse.Code, refreshResponse.Body)
	}
	var refreshed struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(refreshResponse.Body.Bytes(), &refreshed); err != nil {
		t.Fatal(err)
	}
	if refreshed.Token == "" || refreshed.Token == enrolledToken {
		t.Fatalf("token was not rotated: %q", refreshed.Token)
	}

	sessions, err := app.FindRecordsByFilter("agent_sessions", "node = {:node}", "", 10, 0, mapParams("node", node.Id))
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %d, err = %v", len(sessions), err)
	}
	if centercrypto.VerifySecret(sessions[0].GetString("token_hash"), enrolledToken) {
		t.Fatal("old token still matches stored hash")
	}
	if !centercrypto.VerifySecret(sessions[0].GetString("token_hash"), refreshed.Token) {
		t.Fatal("new token does not match stored hash")
	}
}

func enrollAndGetToken(t *testing.T, app core.App, node *core.Record, secret string) string {
	t.Helper()

	response := performJSONRequest(t, app, agentapi.HandleEnroll, nil, map[string]any{
		"node_key":      node.GetString("node_key"),
		"enroll_secret": secret,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("enroll status = %d, body = %s", response.Code, response.Body)
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Token
}

func assertInvalidCredentials(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	if got := response.Body.String(); got != "{\"message\":\"invalid credentials\"}\n" {
		t.Fatalf("body = %q", got)
	}
}

func newEnrollTestApp(t *testing.T) (*tests.TestApp, *core.Record, string) {
	t.Helper()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	if err := collections.EnsureCollections(app); err != nil {
		t.Fatal(err)
	}

	secret, hash, err := centercrypto.GenerateEnrollSecret()
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := app.FindCollectionByNameOrId("nodes")
	if err != nil {
		t.Fatal(err)
	}
	node := core.NewRecord(nodes)
	node.Set("node_key", "node-001")
	node.Set("enroll_secret_hash", hash)
	node.Set("enroll_status", "active")
	node.Set("role", "unknown")
	if err := app.Save(node); err != nil {
		t.Fatal(err)
	}
	return app, node, secret
}

func performJSONRequest(
	t *testing.T,
	app core.App,
	handler func(*core.RequestEvent) error,
	headers map[string]string,
	body map[string]any,
) *httptest.ResponseRecorder {
	t.Helper()

	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	event := &core.RequestEvent{App: app}
	event.Request = request
	event.Response = response
	if err := handler(event); err != nil {
		t.Fatal(err)
	}
	return response
}

func assertAuditCount(t *testing.T, app core.App, nodeID string, want int) {
	t.Helper()
	records, err := app.FindRecordsByFilter(
		"audit_logs",
		"action = 'agent.enroll' && node = {:node}",
		"",
		10,
		0,
		mapParams("node", nodeID),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != want {
		t.Fatalf("audit count = %d, want %d", len(records), want)
	}
}

func mapParams(key string, value any) map[string]any {
	return map[string]any{key: value}
}
