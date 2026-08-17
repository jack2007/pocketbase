package centerapi

import (
	"context"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/p2p"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

type ProxyRequester interface {
	RequestProxy(context.Context, string, agenthub.ProxyRequest) (agenthub.ProxyResponse, error)
}

// API exposes the superuser-only center management handlers.
type API struct {
	hub        ProxyRequester
	agentHub   *agenthub.Hub
	p2p        *p2p.Broker
	startApply func(string) error
}

func New(hub *agenthub.Hub) *API {
	return &API{hub: hub, agentHub: hub}
}

func (api *API) SetP2P(broker *p2p.Broker) {
	api.p2p = broker
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
