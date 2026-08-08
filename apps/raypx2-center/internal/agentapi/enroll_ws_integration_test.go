package agentapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agentapi"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/centerapi"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/protocol"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func TestRevokeRejectsEnrollAndDisconnectsWebSocket(t *testing.T) {
	app, node, secret := newEnrollTestApp(t)
	server, hub := newEnrollWSIntegrationServer(t, app)
	defer server.Close()

	conn := enrollAndConnectWS(t, server.URL, node.GetString("node_key"), secret)
	defer conn.CloseNow()
	waitForHubConnection(t, hub, node.GetString("node_key"))

	response := postIntegrationJSON(t, server.URL+"/api/center/nodes/"+node.GetString("node_key")+"/revoke", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("revoke status = %d, body = %s", response.StatusCode, readResponseBody(t, response))
	}
	assertByeAndClosed(t, conn, "revoked")

	response = postIntegrationJSON(t, server.URL+"/api/agent/enroll", map[string]any{
		"node_key":      node.GetString("node_key"),
		"enroll_secret": secret,
	})
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("enroll after revoke status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
}

func TestRotateEnrollInvalidatesOldSecretAndDisconnectsWebSocket(t *testing.T) {
	app, node, oldSecret := newEnrollTestApp(t)
	server, hub := newEnrollWSIntegrationServer(t, app)
	defer server.Close()

	conn := enrollAndConnectWS(t, server.URL, node.GetString("node_key"), oldSecret)
	defer conn.CloseNow()
	waitForHubConnection(t, hub, node.GetString("node_key"))

	response := postIntegrationJSON(t, server.URL+"/api/center/nodes/"+node.GetString("node_key")+"/rotate-enroll", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("rotate status = %d, body = %s", response.StatusCode, readResponseBody(t, response))
	}
	var rotated struct {
		EnrollSecret string `json:"enroll_secret"`
	}
	if err := json.NewDecoder(response.Body).Decode(&rotated); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if rotated.EnrollSecret == "" || rotated.EnrollSecret == oldSecret {
		t.Fatalf("invalid rotated secret %q", rotated.EnrollSecret)
	}
	assertByeAndClosed(t, conn, "rotated")

	oldResponse := postIntegrationJSON(t, server.URL+"/api/agent/enroll", map[string]any{
		"node_key":      node.GetString("node_key"),
		"enroll_secret": oldSecret,
	})
	defer oldResponse.Body.Close()
	if oldResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old secret enroll status = %d, want %d", oldResponse.StatusCode, http.StatusUnauthorized)
	}

	newResponse := postIntegrationJSON(t, server.URL+"/api/agent/enroll", map[string]any{
		"node_key":      node.GetString("node_key"),
		"enroll_secret": rotated.EnrollSecret,
	})
	defer newResponse.Body.Close()
	if newResponse.StatusCode != http.StatusOK {
		t.Fatalf("new secret enroll status = %d, body = %s", newResponse.StatusCode, readResponseBody(t, newResponse))
	}
}

func newEnrollWSIntegrationServer(t *testing.T, app core.App) (*httptest.Server, *agenthub.Hub) {
	t.Helper()
	hub := agenthub.New(agenthub.WithSessionRevoker(func(sessionID string) error {
		return agentapi.RevokeSession(app, sessionID)
	}))
	agentapi.SetHub(hub)
	t.Cleanup(func() { agentapi.SetHub(agenthub.New()) })
	api := centerapi.New(hub)

	pbRouter := router.NewRouter(func(w http.ResponseWriter, r *http.Request) (*core.RequestEvent, router.EventCleanupFunc) {
		event := &core.RequestEvent{App: app}
		event.Response = w
		event.Request = r
		return event, nil
	})
	pbRouter.POST("/api/agent/enroll", agentapi.HandleEnroll)
	pbRouter.GET("/api/agent/ws", agentapi.HandleWS)
	pbRouter.POST("/api/center/nodes/{node_key}/rotate-enroll", api.HandleRotateEnroll)
	pbRouter.POST("/api/center/nodes/{node_key}/revoke", api.HandleRevokeNode)
	mux, err := pbRouter.BuildMux()
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(mux), hub
}

func enrollAndConnectWS(t *testing.T, baseURL, nodeKey, secret string) *websocket.Conn {
	t.Helper()
	response := postIntegrationJSON(t, baseURL+"/api/agent/enroll", map[string]any{
		"node_key":      nodeKey,
		"enroll_secret": secret,
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("enroll status = %d, body = %s", response.StatusCode, readResponseBody(t, response))
	}
	var enrolled struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&enrolled); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/api/agent/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + enrolled.Token}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var welcome protocol.Frame
	if err := wsjson.Read(ctx, conn, &welcome); err != nil {
		conn.CloseNow()
		t.Fatal(err)
	}
	if welcome.Type != "welcome" {
		conn.CloseNow()
		t.Fatalf("first frame type = %q, want welcome", welcome.Type)
	}
	return conn
}

func waitForHubConnection(t *testing.T, hub *agenthub.Hub, nodeKey string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.HasConnection(nodeKey) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("websocket was not registered in hub")
}

func assertByeAndClosed(t *testing.T, conn *websocket.Conn, reason string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var bye protocol.Frame
	if err := wsjson.Read(ctx, conn, &bye); err != nil {
		t.Fatalf("read bye: %v", err)
	}
	var payload struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(bye.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if bye.Type != "bye" || payload.Reason != reason {
		t.Fatalf("bye = %#v, reason = %q", bye, payload.Reason)
	}
	if err := wsjson.Read(ctx, conn, &bye); err == nil {
		t.Fatal("websocket remained open after bye")
	}
}

func postIntegrationJSON(t *testing.T, url string, body map[string]any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequest(http.MethodPost, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func readResponseBody(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
