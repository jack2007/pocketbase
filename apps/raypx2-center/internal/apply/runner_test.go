package apply

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/collections"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

type fakeProxy struct {
	configs map[string]map[string]any
	writes  map[string]map[string]any
	fail    map[string]error
}

func (p *fakeProxy) RequestProxy(_ context.Context, nodeKey string, request agenthub.ProxyRequest) (agenthub.ProxyResponse, error) {
	if err := p.fail[nodeKey]; err != nil {
		return agenthub.ProxyResponse{}, err
	}
	if request.Method == http.MethodGet {
		body, _ := json.Marshal(p.configs[nodeKey])
		return agenthub.ProxyResponse{Status: http.StatusOK, BodyB64: encode(body)}, nil
	}
	body, err := decodeRequest(request)
	if err != nil {
		return agenthub.ProxyResponse{}, err
	}
	p.writes[nodeKey] = body
	response, _ := json.Marshal(body)
	return agenthub.ProxyResponse{Status: http.StatusOK, BodyB64: encode(response)}, nil
}

func encode(value []byte) string {
	return base64.StdEncoding.EncodeToString(value)
}

func decodeRequest(request agenthub.ProxyRequest) (map[string]any, error) {
	encoded, err := base64.StdEncoding.DecodeString(request.BodyB64)
	if err != nil {
		return nil, err
	}
	var body map[string]any
	err = json.Unmarshal(encoded, &body)
	return body, err
}

func containsSecret(value []byte) bool {
	return bytes.Contains(value, []byte("SECRET"))
}

func TestRunnerAppliesTemplateAndRecordsRevisions(t *testing.T) {
	app, job, targets := newApplyFixture(t)
	proxy := &fakeProxy{
		configs: map[string]map[string]any{
			"server-a": {"role": "server", "allow_targets": []any{"10.0.0.0/8"}, "admin_token": "SECRET"},
			"server-b": {"role": "server", "allow_targets": []any{"10.0.0.0/8"}},
		},
		writes: map[string]map[string]any{},
		fail:   map[string]error{"server-b": agenthub.ErrNodeOffline},
	}

	if err := NewRunner(app, proxy).RunJob(context.Background(), job.Id); err != nil {
		t.Fatal(err)
	}

	storedJob, err := app.FindRecordById("apply_jobs", job.Id)
	if err != nil {
		t.Fatal(err)
	}
	if storedJob.GetString("status") != "partial" {
		t.Fatalf("job status = %q", storedJob.GetString("status"))
	}
	first, _ := app.FindRecordById("apply_job_targets", targets[0].Id)
	second, _ := app.FindRecordById("apply_job_targets", targets[1].Id)
	if first.GetString("status") != "completed" || second.GetString("status") != "failed" {
		t.Fatalf("target statuses = %q, %q", first.GetString("status"), second.GetString("status"))
	}
	if first.GetString("result_revision") == "" || second.GetString("error") != "node_offline" {
		t.Fatalf("target results = %#v, %#v", first, second)
	}
	if got := proxy.writes["server-a"]["allow_targets"]; len(got.([]any)) != 1 || got.([]any)[0] != "127.0.0.0/8" {
		t.Fatalf("write = %#v", proxy.writes["server-a"])
	}
	revisions, err := app.FindRecordsByFilter("config_revisions", "", "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 {
		t.Fatalf("revisions = %d, want actual and desired", len(revisions))
	}
	encoded, _ := json.Marshal(revisions[0].Get("content"))
	if string(encoded) == "" || containsSecret(encoded) {
		t.Fatalf("revision leaked secret: %s", encoded)
	}
}

func TestRunnerMarksJobFailedWhenEveryTargetFails(t *testing.T) {
	app, job, _ := newApplyFixture(t)
	proxy := &fakeProxy{
		configs: map[string]map[string]any{},
		writes:  map[string]map[string]any{},
		fail: map[string]error{
			"server-a": errors.New("boom"),
			"server-b": errors.New("boom"),
		},
	}
	if err := NewRunner(app, proxy).RunJob(context.Background(), job.Id); err != nil {
		t.Fatal(err)
	}
	stored, _ := app.FindRecordById("apply_jobs", job.Id)
	if stored.GetString("status") != "failed" {
		t.Fatalf("job status = %q", stored.GetString("status"))
	}
}

func newApplyFixture(t *testing.T) (*tests.TestApp, *core.Record, []*core.Record) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	if err := collections.EnsureCollections(app); err != nil {
		t.Fatal(err)
	}
	nodes, _ := app.FindCollectionByNameOrId("nodes")
	var nodeRecords []*core.Record
	for _, key := range []string{"server-a", "server-b"} {
		node := core.NewRecord(nodes)
		node.Set("node_key", key)
		node.Set("role", "server")
		node.Set("enroll_secret_hash", "hash")
		node.Set("enroll_status", "active")
		if err := app.Save(node); err != nil {
			t.Fatal(err)
		}
		nodeRecords = append(nodeRecords, node)
	}
	templates, _ := app.FindCollectionByNameOrId("config_templates")
	template := core.NewRecord(templates)
	template.Set("name", "lock down")
	template.Set("target_role", "server")
	template.Set("version", 1)
	template.Set("body", map[string]any{"allow_targets": []any{"127.0.0.0/8"}})
	if err := app.Save(template); err != nil {
		t.Fatal(err)
	}
	jobs, _ := app.FindCollectionByNameOrId("apply_jobs")
	job := core.NewRecord(jobs)
	job.Set("template", template.Id)
	job.Set("template_version", 1)
	job.Set("status", "queued")
	if err := app.Save(job); err != nil {
		t.Fatal(err)
	}
	targetCollection, _ := app.FindCollectionByNameOrId("apply_job_targets")
	var targets []*core.Record
	for _, node := range nodeRecords {
		target := core.NewRecord(targetCollection)
		target.Set("job", job.Id)
		target.Set("node", node.Id)
		target.Set("status", "queued")
		if err := app.Save(target); err != nil {
			t.Fatal(err)
		}
		targets = append(targets, target)
	}
	return app, job, targets
}
