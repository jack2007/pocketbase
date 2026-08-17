package centerapi

import (
	"net/http"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/p2p"
	"github.com/pocketbase/pocketbase/core"
)

type createP2PSessionRequest struct {
	ClientNodeKey string `json:"client_node_key"`
	ServerNodeKey string `json:"server_node_key"`
	ConnectionID  string `json:"connection_id"`
}

// HandleCreateP2PSession issues invite/grant to two online agents.
func (api *API) HandleCreateP2PSession(e *core.RequestEvent) error {
	if api.p2p == nil {
		return e.JSON(http.StatusServiceUnavailable, errorResponse("p2p_unavailable"))
	}
	var request createP2PSessionRequest
	if err := e.BindBody(&request); err != nil {
		return e.JSON(http.StatusBadRequest, errorResponse("invalid_request"))
	}
	server, err := e.App.FindFirstRecordByData("nodes", "node_key", request.ServerNodeKey)
	if err != nil {
		return e.JSON(http.StatusNotFound, errorResponse("server_node_not_found"))
	}
	if _, err := e.App.FindFirstRecordByData("nodes", "node_key", request.ClientNodeKey); err != nil {
		return e.JSON(http.StatusNotFound, errorResponse("client_node_not_found"))
	}
	key, err := p2p.VerifyKeyFromHex(server.GetString("grant_mac_key"))
	if err != nil {
		return e.JSON(http.StatusConflict, errorResponse("server_grant_key_missing"))
	}
	session, err := api.p2p.Create(p2p.CreateRequest{
		ClientNodeKey: request.ClientNodeKey,
		ServerNodeKey: request.ServerNodeKey,
		ConnectionID:  request.ConnectionID,
		GrantMACKey:   key,
	})
	if err != nil {
		return e.JSON(http.StatusConflict, errorResponse(err.Error()))
	}
	return e.JSON(http.StatusCreated, map[string]any{
		"session_id":    session.SessionID,
		"connection_id": session.ConnectionID,
		"epoch":         session.Epoch,
	})
}
