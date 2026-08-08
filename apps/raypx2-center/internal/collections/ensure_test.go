package collections_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/collections"
	"github.com/pocketbase/pocketbase/tests"
)

func TestEnsureCollectionsCreatesNodes(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := collections.EnsureCollections(app); err != nil {
		t.Fatal(err)
	}

	wantCollections := []string{
		"nodes",
		"agent_sessions",
		"node_status",
		"config_revisions",
		"config_templates",
		"apply_jobs",
		"apply_job_targets",
		"audit_logs",
	}
	for _, name := range wantCollections {
		if _, err := app.FindCollectionByNameOrId(name); err != nil {
			t.Fatalf("collection %q missing: %v", name, err)
		}
	}

	nodes, err := app.FindCollectionByNameOrId("nodes")
	if err != nil {
		t.Fatalf("nodes lookup failed: %v", err)
	}
	if nodes.ListRule != nil {
		t.Fatalf("nodes.ListRule = %q, want nil (superuser-only default)", *nodes.ListRule)
	}
}
