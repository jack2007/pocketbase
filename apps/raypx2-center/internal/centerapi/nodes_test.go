package centerapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

func TestRevokeNodeSessionsRevokesMoreThanOnePage(t *testing.T) {
	app, node, _ := newCenterTestApp(t)
	collection, err := app.FindCollectionByNameOrId("agent_sessions")
	if err != nil {
		t.Fatal(err)
	}
	const total = 1001
	for i := range total {
		session := core.NewRecord(collection)
		session.Set("node", node.Id)
		session.Set("token_hash", fmt.Sprintf("hash-%d", i))
		session.Set("expires_at", types.NowDateTime().Add(time.Hour))
		if err := app.Save(session); err != nil {
			t.Fatal(err)
		}
	}

	if err := revokeNodeSessions(app, node.Id); err != nil {
		t.Fatal(err)
	}
	active, err := app.FindRecordsByFilter(
		"agent_sessions",
		"node = {:node} && revoked_at = ''",
		"",
		1,
		0,
		map[string]any{"node": node.Id},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatal("at least one session beyond the first page remained active")
	}
}

func TestRotateAndRevokeNodeAreAuditedWithoutSecrets(t *testing.T) {
	app, node, auth := newCenterTestApp(t)
	api := New(agenthub.New())
	rotated := performCenterRequest(
		t,
		app,
		auth,
		http.MethodPost,
		"/api/center/nodes/"+node.GetString("node_key")+"/rotate-enroll",
		node.GetString("node_key"),
		nil,
		api.HandleRotateEnroll,
	)
	if rotated.Code != http.StatusOK {
		t.Fatalf("rotate response = %d %s", rotated.Code, rotated.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rotated.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	secret, _ := response["enroll_secret"].(string)

	revoked := performCenterRequest(
		t,
		app,
		auth,
		http.MethodPost,
		"/api/center/nodes/"+node.GetString("node_key")+"/revoke",
		node.GetString("node_key"),
		nil,
		api.HandleRevokeNode,
	)
	if revoked.Code != http.StatusOK {
		t.Fatalf("revoke response = %d %s", revoked.Code, revoked.Body.String())
	}

	for _, action := range []string{"node.rotate_enroll", "node.revoke"} {
		logs, err := app.FindRecordsByFilter(
			"audit_logs",
			"action = {:action} && node = {:node}",
			"",
			10,
			0,
			map[string]any{"action": action, "node": node.Id},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(logs) != 1 || logs[0].GetString("actor") != auth.Id {
			t.Fatalf("%s audits = %#v", action, logs)
		}
		summary, err := json.Marshal(logs[0].Get("request_summary"))
		if err != nil {
			t.Fatal(err)
		}
		if secret != "" && bytes.Contains(summary, []byte(secret)) {
			t.Fatalf("%s audit leaked enrollment secret: %s", action, summary)
		}
	}
}
