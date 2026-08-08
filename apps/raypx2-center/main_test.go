package main

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/collections"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestEmbeddedCenterUIServesAppIndex(t *testing.T) {
	scenario := tests.ApiScenario{
		Name:            "GET /app/ serves embedded SPA",
		Method:          http.MethodGet,
		URL:             "/app/",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"<title>raypx2 center</title>", "/app/assets/"},
		ExpectedEvents:  map[string]int{"*": 0},
		BeforeTestFunc: func(t testing.TB, _ *tests.TestApp, e *core.ServeEvent) {
			bindUIRoutes(e)
		},
	}
	scenario.Test(t)
}

func TestInitializeCenterMarksAllNodesOffline(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	if err := collections.EnsureCollections(app); err != nil {
		t.Fatal(err)
	}
	nodes, err := app.FindCollectionByNameOrId("nodes")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"online-a", "online-b"} {
		node := core.NewRecord(nodes)
		node.Set("node_key", key)
		node.Set("enroll_secret_hash", "hash")
		node.Set("enroll_status", "active")
		node.Set("role", "unknown")
		node.Set("online", true)
		if err := app.Save(node); err != nil {
			t.Fatal(err)
		}
	}

	if err := initializeCenter(app); err != nil {
		t.Fatal(err)
	}
	stored, err := app.FindAllRecords("nodes")
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range stored {
		if node.GetBool("online") {
			t.Fatalf("node %q remained online after startup", node.GetString("node_key"))
		}
	}
}
