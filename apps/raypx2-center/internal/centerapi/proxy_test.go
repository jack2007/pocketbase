package centerapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/collections"
	centercrypto "github.com/pocketbase/pocketbase/apps/raypx2-center/internal/crypto"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/protocol"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

type replyingConn struct {
	hub     *agenthub.Hub
	nodeKey string
	request agenthub.ProxyRequest
}

func (c *replyingConn) ID() string         { return "fake-agent" }
func (c *replyingConn) SessionID() string  { return "fake-session" }
func (c *replyingConn) Close(string) error { return nil }

func (c *replyingConn) Send(_ context.Context, frame protocol.Frame) error {
	if frame.Type != "http_proxy_req" {
		return nil
	}
	if err := json.Unmarshal(frame.Payload, &c.request); err != nil {
		return err
	}
	payload, err := json.Marshal(agenthub.ProxyResponse{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		BodyB64: base64.StdEncoding.EncodeToString([]byte(`{"status":"healthy"}`)),
	})
	if err != nil {
		return err
	}
	c.hub.HandleFrame(c.nodeKey, protocol.Frame{
		Type:    "http_proxy_res",
		ID:      frame.ID,
		Payload: payload,
	})
	return nil
}

func TestProxyRoundTrip(t *testing.T) {
	app, node, auth := newCenterTestApp(t)
	hub := agenthub.New()
	conn := &replyingConn{hub: hub, nodeKey: node.GetString("node_key")}
	hub.Register(conn.nodeKey, conn)
	api := New(hub)

	response := performCenterRequest(t, app, auth, http.MethodPost,
		"/api/center/nodes/"+conn.nodeKey+"/proxy", conn.nodeKey, map[string]any{
			"method": "GET",
			"path":   "/api/v1/health",
			"headers": map[string]string{
				"X-Request-ID": "test-request",
			},
		}, api.HandleProxy)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := response.Body.String(); got != `{"status":"healthy"}` {
		t.Fatalf("body = %s", got)
	}
	if conn.request.Method != "GET" || conn.request.Path != "/api/v1/health" ||
		conn.request.TimeoutMS != 10_000 {
		t.Fatalf("proxy request = %#v", conn.request)
	}

	logs, err := app.FindRecordsByFilter(
		"audit_logs",
		"action = 'proxy.request' && node = {:node}",
		"",
		10,
		0,
		map[string]any{"node": node.Id},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("audit logs = %d, want 1", len(logs))
	}
	summary, err := json.Marshal(logs[0].Get("request_summary"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(summary, []byte("secret")) || !bytes.Contains(summary, []byte(`"status":200`)) {
		t.Fatalf("unsafe or incomplete audit summary: %s", summary)
	}
}

func TestProxyRejectsInvalidPathAndReportsOffline(t *testing.T) {
	app, node, auth := newCenterTestApp(t)
	api := New(agenthub.New())

	invalid := performCenterRequest(t, app, auth, http.MethodPost,
		"/api/center/nodes/"+node.GetString("node_key")+"/proxy",
		node.GetString("node_key"),
		map[string]any{"method": "GET", "path": "/metrics"},
		api.HandleProxy)
	if invalid.Code != http.StatusBadRequest ||
		!bytes.Contains(invalid.Body.Bytes(), []byte("invalid_proxy_path")) {
		t.Fatalf("invalid response = %d %s", invalid.Code, invalid.Body.String())
	}

	offline := performCenterRequest(t, app, auth, http.MethodPost,
		"/api/center/nodes/"+node.GetString("node_key")+"/proxy",
		node.GetString("node_key"),
		map[string]any{"method": "GET", "path": "/api/v1/health"},
		api.HandleProxy)
	if offline.Code != http.StatusServiceUnavailable ||
		!bytes.Contains(offline.Body.Bytes(), []byte("node_offline")) {
		t.Fatalf("offline response = %d %s", offline.Code, offline.Body.String())
	}
}

func TestCreateAndListNodesKeepsEnrollSecretOneTimeOnly(t *testing.T) {
	app, _, auth := newCenterTestApp(t)
	api := New(agenthub.New())

	created := performCenterRequest(t, app, auth, http.MethodPost,
		"/api/center/nodes", "", map[string]any{
			"node_key": "created-node",
			"name":     "Created node",
			"role":     "server",
			"labels":   map[string]string{"region": "test"},
		}, api.HandleCreateNode)
	if created.Code != http.StatusCreated {
		t.Fatalf("create response = %d %s", created.Code, created.Body.String())
	}
	var result struct {
		Node         map[string]any `json:"node"`
		EnrollSecret string         `json:"enroll_secret"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.EnrollSecret == "" || result.Node["enroll_secret_hash"] != nil {
		t.Fatalf("create result leaked hash or omitted secret: %#v", result)
	}
	stored, err := app.FindFirstRecordByData("nodes", "node_key", "created-node")
	if err != nil {
		t.Fatal(err)
	}
	if !centercrypto.VerifySecret(stored.GetString("enroll_secret_hash"), result.EnrollSecret) {
		t.Fatal("returned enrollment secret does not match stored hash")
	}

	listed := performCenterRequest(t, app, auth, http.MethodGet,
		"/api/center/nodes", "", nil, api.HandleListNodes)
	if listed.Code != http.StatusOK {
		t.Fatalf("list response = %d %s", listed.Code, listed.Body.String())
	}
	if bytes.Contains(listed.Body.Bytes(), []byte(result.EnrollSecret)) ||
		bytes.Contains(listed.Body.Bytes(), []byte("enroll_secret_hash")) {
		t.Fatalf("list leaked enrollment secret: %s", listed.Body.String())
	}
}

func newCenterTestApp(t *testing.T) (*tests.TestApp, *core.Record, *core.Record) {
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
	node.Set("node_key", "node-center-test")
	node.Set("enroll_secret_hash", hash)
	node.Set("enroll_status", "active")
	node.Set("role", "unknown")
	if err := app.Save(node); err != nil {
		t.Fatal(err)
	}
	auth, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	return app, node, auth
}

func performCenterRequest(
	t *testing.T,
	app core.App,
	auth *core.Record,
	method string,
	path string,
	nodeKey string,
	body map[string]any,
	handler func(*core.RequestEvent) error,
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
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	if nodeKey != "" {
		request.SetPathValue("node_key", nodeKey)
	}
	response := httptest.NewRecorder()
	event := &core.RequestEvent{App: app, Auth: auth}
	event.Request = request
	event.Response = response
	if err := handler(event); err != nil {
		t.Fatal(err)
	}
	return response
}
