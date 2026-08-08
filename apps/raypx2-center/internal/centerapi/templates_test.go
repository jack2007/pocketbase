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

func TestTemplateAPIRejectsSecretsAndVersionsUpdates(t *testing.T) {
	app, _, auth := newCenterTestApp(t)
	api := New(agenthub.New())

	rejected := performCenterRequest(t, app, auth, http.MethodPost,
		"/api/center/templates", "", map[string]any{
			"name": "unsafe", "target_role": "client",
			"body": map[string]any{"peers": []any{map[string]any{
				"peer_id": "peer-a", "tls": map[string]any{"key": "SECRET"},
			}}},
		}, api.HandleCreateTemplate)
	if rejected.Code != http.StatusBadRequest || !bytes.Contains(rejected.Body.Bytes(), []byte("invalid_template")) {
		t.Fatalf("unsafe template response = %d %s", rejected.Code, rejected.Body.String())
	}

	created := performCenterRequest(t, app, auth, http.MethodPost,
		"/api/center/templates", "", map[string]any{
			"name": "safe", "target_role": "server",
			"body": map[string]any{"deny_targets": []any{"169.254.0.0/16"}},
		}, api.HandleCreateTemplate)
	if created.Code != http.StatusCreated {
		t.Fatalf("create response = %d %s", created.Code, created.Body.String())
	}
	templates, err := app.FindRecordsByFilter("config_templates", "name = 'safe'", "", 1, 0)
	if err != nil || len(templates) != 1 {
		t.Fatalf("templates = %d, err = %v", len(templates), err)
	}
	if templates[0].GetInt("version") != 1 {
		t.Fatalf("initial version = %d", templates[0].GetInt("version"))
	}

	updated := performRequestWithPathValue(t, app, auth, http.MethodPut,
		"/api/center/templates/"+templates[0].Id, "template_id", templates[0].Id,
		map[string]any{"name": "safe v2", "target_role": "server", "body": map[string]any{"allow_targets": []any{}}},
		api.HandleUpdateTemplate)
	if updated.Code != http.StatusOK {
		t.Fatalf("update response = %d %s", updated.Code, updated.Body.String())
	}
	stored, _ := app.FindRecordById("config_templates", templates[0].Id)
	if stored.GetInt("version") != 2 {
		t.Fatalf("updated version = %d", stored.GetInt("version"))
	}
}

func TestCreateApplyJobCreatesTargets(t *testing.T) {
	app, firstNode, auth := newCenterTestApp(t)
	nodes, _ := app.FindCollectionByNameOrId("nodes")
	secondNode := core.NewRecord(nodes)
	secondNode.Set("node_key", "node-center-test-2")
	secondNode.Set("enroll_secret_hash", "hash")
	secondNode.Set("enroll_status", "active")
	secondNode.Set("role", "unknown")
	if err := app.Save(secondNode); err != nil {
		t.Fatal(err)
	}
	templates, _ := app.FindCollectionByNameOrId("config_templates")
	template := core.NewRecord(templates)
	template.Set("name", "client peers")
	template.Set("target_role", "unknown")
	template.Set("version", 1)
	template.Set("body", map[string]any{"peers": []any{}})
	if err := app.Save(template); err != nil {
		t.Fatal(err)
	}

	api := New(agenthub.New())
	started := ""
	api.startApply = func(_ string) error {
		started = "yes"
		return nil
	}
	response := performCenterRequest(t, app, auth, http.MethodPost,
		"/api/center/apply-jobs", "", map[string]any{
			"template": template.Id,
			"nodes":    []any{firstNode.Id, secondNode.Id},
		}, api.HandleCreateApplyJob)
	if response.Code != http.StatusCreated || started != "yes" {
		t.Fatalf("create job = %d %s, started=%s", response.Code, response.Body.String(), started)
	}
	targets, err := app.FindRecordsByFilter("apply_job_targets", "", "", 10, 0)
	if err != nil || len(targets) != 2 {
		t.Fatalf("targets = %d, err = %v", len(targets), err)
	}
}

func performRequestWithPathValue(
	t *testing.T,
	app core.App,
	auth *core.Record,
	method, path, key, value string,
	body map[string]any,
	handler func(*core.RequestEvent) error,
) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.SetPathValue(key, value)
	response := httptest.NewRecorder()
	event := &core.RequestEvent{App: app, Auth: auth}
	event.Request = request
	event.Response = response
	if err := handler(event); err != nil {
		t.Fatal(err)
	}
	return response
}
