package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agentapi"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/apply"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/centerapi"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/collections"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/p2p"
	"github.com/pocketbase/pocketbase/core"
)

//go:embed all:ui/dist
var embeddedUI embed.FS

var uiDist, _ = fs.Sub(embeddedUI, "ui/dist")

func main() {
	app := pocketbase.New()
	hub := agenthub.New(agenthub.WithSessionRevoker(func(sessionID string) error {
		return agentapi.RevokeSession(app, sessionID)
	}))
	agentapi.SetHub(hub)
	apply.SetProxyRequester(hub)
	centerAPI := centerapi.New(hub)
	p2pBroker := p2p.NewBroker(hub, p2p.ConfigFromEnv())
	centerAPI.SetP2P(p2pBroker)
	agentapi.SetP2PBroker(p2pBroker)

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if err := initializeCenter(e.App); err != nil {
			return err
		}
		e.Router.POST("/api/agent/enroll", agentapi.HandleEnroll)
		e.Router.POST("/api/agent/session/refresh", agentapi.HandleRefresh)
		e.Router.GET("/api/agent/ws", agentapi.HandleWS)
		center := e.Router.Group("/api/center").Bind(centerapi.RequireSuperuserAuth())
		center.POST("/nodes", centerAPI.HandleCreateNode)
		center.GET("/nodes", centerAPI.HandleListNodes)
		center.DELETE("/nodes/{node_key}", centerAPI.HandleDeleteNode)
		center.POST("/nodes/{node_key}/rotate-enroll", centerAPI.HandleRotateEnroll)
		center.POST("/nodes/{node_key}/revoke", centerAPI.HandleRevokeNode)
		center.POST("/nodes/{node_key}/proxy", centerAPI.HandleProxy)
		center.GET("/nodes/{node_key}/config", centerAPI.HandleGetNodeConfig)
		center.PUT("/nodes/{node_key}/config", centerAPI.HandlePutNodeConfig)
		center.DELETE("/nodes/{node_key}/peers/{peer_id}", centerAPI.HandleDeleteNodePeer)
		center.GET("/templates", centerAPI.HandleListTemplates)
		center.POST("/templates", centerAPI.HandleCreateTemplate)
		center.PUT("/templates/{template_id}", centerAPI.HandleUpdateTemplate)
		center.DELETE("/templates/{template_id}", centerAPI.HandleDeleteTemplate)
		center.GET("/apply-jobs", centerAPI.HandleListApplyJobs)
		center.POST("/apply-jobs", centerAPI.HandleCreateApplyJob)
		center.GET("/apply-jobs/{job_id}", centerAPI.HandleGetApplyJob)
		center.POST("/p2p/sessions", centerAPI.HandleCreateP2PSession)
		bindUIRoutes(e)
		return e.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

func initializeCenter(app core.App) error {
	if err := collections.EnsureCollections(app); err != nil {
		return err
	}
	nodes, err := app.FindAllRecords("nodes")
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if !node.GetBool("online") {
			continue
		}
		node.Set("online", false)
		if err := app.Save(node); err != nil {
			return err
		}
	}
	return nil
}

func bindUIRoutes(e *core.ServeEvent) {
	e.Router.GET("/app/{path...}", apis.Static(uiDist, true))
}
