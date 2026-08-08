package main

import (
	"net/http"
	"testing"

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
