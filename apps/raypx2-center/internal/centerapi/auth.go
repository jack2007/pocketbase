package centerapi

import (
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

// API exposes the superuser-only center management handlers.
type API struct {
	hub        *agenthub.Hub
	startApply func(string) error
}

func New(hub *agenthub.Hub) *API {
	return &API{hub: hub}
}

func actorID(e *core.RequestEvent) string {
	if e.Auth == nil {
		return ""
	}
	return e.Auth.Id
}

// RequireSuperuserAuth uses PocketBase's standard superuser auth middleware.
func RequireSuperuserAuth() *hook.Handler[*core.RequestEvent] {
	return apis.RequireSuperuserAuth()
}
