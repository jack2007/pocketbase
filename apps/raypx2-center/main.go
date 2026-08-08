package main

import (
	"log"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agentapi"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/centerapi"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/collections"
	"github.com/pocketbase/pocketbase/core"
)

func main() {
	app := pocketbase.New()
	hub := agenthub.New(agenthub.WithSessionRevoker(func(sessionID string) error {
		return agentapi.RevokeSession(app, sessionID)
	}))
	agentapi.SetHub(hub)
	centerAPI := centerapi.New(hub)

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if err := collections.EnsureCollections(e.App); err != nil {
			return err
		}
		e.Router.POST("/api/agent/enroll", agentapi.HandleEnroll)
		e.Router.POST("/api/agent/session/refresh", agentapi.HandleRefresh)
		e.Router.GET("/api/agent/ws", agentapi.HandleWS)
		center := e.Router.Group("/api/center").Bind(centerapi.RequireSuperuserAuth())
		center.POST("/nodes", centerAPI.HandleCreateNode)
		center.GET("/nodes", centerAPI.HandleListNodes)
		center.POST("/nodes/{node_key}/proxy", centerAPI.HandleProxy)
		return e.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
