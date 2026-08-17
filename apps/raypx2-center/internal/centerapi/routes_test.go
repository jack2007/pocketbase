package centerapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/collections"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestCenterRoutesRequireSuperuserAuth(t *testing.T) {
	t.Parallel()

	type scenario struct {
		name           string
		method         string
		url            string
		body           string
		authenticated  bool
		expectedStatus int
		expected       []string
		notExpected    []string
	}

	for _, base := range []scenario{
		{
			name:           "unauthenticated GET /api/center/nodes",
			method:         http.MethodGet,
			url:            "/api/center/nodes",
			expectedStatus: http.StatusUnauthorized,
			expected:       []string{`"data":{}`},
		},
		{
			name:   "unauthenticated POST /api/center/nodes",
			method: http.MethodPost,
			url:    "/api/center/nodes",
			body: `{
				"node_key": "route-auth-test",
				"name": "Route auth test",
				"role": "server"
			}`,
			expectedStatus: http.StatusUnauthorized,
			expected:       []string{`"data":{}`},
		},
		{
			name:           "superuser GET /api/center/nodes",
			method:         http.MethodGet,
			url:            "/api/center/nodes",
			authenticated:  true,
			expectedStatus: http.StatusOK,
			expected:       []string{`"items"`},
		},
		{
			name:           "unauthenticated GET /api/center/nodes/{node_key}/config",
			method:         http.MethodGet,
			url:            "/api/center/nodes/missing/config",
			expectedStatus: http.StatusUnauthorized,
			expected:       []string{`"data":{}`},
		},
		{
			name:           "unauthenticated PUT /api/center/nodes/{node_key}/config",
			method:         http.MethodPut,
			url:            "/api/center/nodes/missing/config",
			body:           `{"content":{"allow_targets":["127.0.0.0/8"]}}`,
			expectedStatus: http.StatusUnauthorized,
			expected:       []string{`"data":{}`},
		},
		{
			name:           "unauthenticated DELETE /api/center/nodes/{node_key}/peers/{peer_id}",
			method:         http.MethodDelete,
			url:            "/api/center/nodes/missing/peers/peer-a",
			expectedStatus: http.StatusUnauthorized,
			expected:       []string{`"data":{}`},
		},
		{
			name:           "unauthenticated GET /api/center/templates",
			method:         http.MethodGet,
			url:            "/api/center/templates",
			expectedStatus: http.StatusUnauthorized,
			expected:       []string{`"data":{}`},
		},
		{
			name:           "superuser GET /api/center/templates",
			method:         http.MethodGet,
			url:            "/api/center/templates",
			authenticated:  true,
			expectedStatus: http.StatusOK,
			expected:       []string{`"items"`},
		},
		{
			name:           "unauthenticated GET /api/center/apply-jobs",
			method:         http.MethodGet,
			url:            "/api/center/apply-jobs",
			expectedStatus: http.StatusUnauthorized,
			expected:       []string{`"data":{}`},
		},
		{
			name:           "unauthenticated POST /api/center/p2p/sessions",
			method:         http.MethodPost,
			url:            "/api/center/p2p/sessions",
			body:           `{"client_node_key":"c","server_node_key":"s"}`,
			expectedStatus: http.StatusUnauthorized,
			expected:       []string{`"data":{}`},
		},
		{
			name:           "superuser GET /api/center/apply-jobs",
			method:         http.MethodGet,
			url:            "/api/center/apply-jobs",
			authenticated:  true,
			expectedStatus: http.StatusOK,
			expected:       []string{`"items"`},
		},
		{
			name:   "superuser POST /api/center/nodes",
			method: http.MethodPost,
			url:    "/api/center/nodes",
			body: `{
				"node_key": "route-auth-created",
				"name": "Route auth created",
				"role": "server"
			}`,
			authenticated:  true,
			expectedStatus: http.StatusCreated,
			expected:       []string{`"node"`, `"enroll_secret"`, `"route-auth-created"`},
			notExpected:    []string{`"enroll_secret_hash"`},
		},
	} {
		base := base
		apiScenario := tests.ApiScenario{
			Name:               base.name,
			Method:             base.method,
			URL:                base.url,
			ExpectedStatus:     base.expectedStatus,
			ExpectedContent:    base.expected,
			NotExpectedContent: base.notExpected,
			ExpectedEvents:     map[string]int{"*": 0},
		}
		if base.body != "" {
			apiScenario.Body = strings.NewReader(base.body)
		}
		if base.authenticated {
			apiScenario.ExpectedEvents = nil
			apiScenario.BeforeTestFunc = func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				bindCenterAPIRoutes(t, app, e)
				token, err := superuserAuthToken(app)
				if err != nil {
					t.Fatal(err)
				}
				apiScenario.Headers = map[string]string{"Authorization": token}
			}
		} else {
			apiScenario.BeforeTestFunc = bindCenterAPIRoutes
		}
		apiScenario.Test(t)
	}
}

func bindCenterAPIRoutes(t testing.TB, _ *tests.TestApp, e *core.ServeEvent) {
	t.Helper()
	if err := collections.EnsureCollections(e.App); err != nil {
		t.Fatal(err)
	}
	api := New(agenthub.New())
	center := e.Router.Group("/api/center").Bind(RequireSuperuserAuth())
	center.POST("/nodes", api.HandleCreateNode)
	center.GET("/nodes", api.HandleListNodes)
	center.DELETE("/nodes/{node_key}", api.HandleDeleteNode)
	center.GET("/nodes/{node_key}/config", api.HandleGetNodeConfig)
	center.PUT("/nodes/{node_key}/config", api.HandlePutNodeConfig)
	center.DELETE("/nodes/{node_key}/peers/{peer_id}", api.HandleDeleteNodePeer)
	center.GET("/templates", api.HandleListTemplates)
	center.GET("/apply-jobs", api.HandleListApplyJobs)
	center.POST("/p2p/sessions", api.HandleCreateP2PSession)
}

func superuserAuthToken(app core.App) (string, error) {
	superuser, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		return "", err
	}
	return superuser.NewAuthToken()
}
