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

	if _, err := app.FindCollectionByNameOrId("nodes"); err != nil {
		t.Fatalf("nodes missing: %v", err)
	}
}
