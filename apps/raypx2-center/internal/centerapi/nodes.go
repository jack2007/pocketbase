package centerapi

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/audit"
	centercrypto "github.com/pocketbase/pocketbase/apps/raypx2-center/internal/crypto"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

type createNodeRequest struct {
	NodeKey string `json:"node_key"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Labels  any    `json:"labels"`
}

// HandleCreateNode creates an active node and returns its enrollment secret once.
func (api *API) HandleCreateNode(e *core.RequestEvent) error {
	var request createNodeRequest
	if err := e.BindBody(&request); err != nil {
		return e.JSON(http.StatusBadRequest, errorResponse("invalid_request"))
	}
	if request.NodeKey == "" {
		request.NodeKey = uuid.NewString()
	}
	if request.Role == "" {
		request.Role = "unknown"
	}

	secret, hash, err := centercrypto.GenerateEnrollSecret()
	if err != nil {
		return e.InternalServerError("Failed to generate enrollment secret.", err)
	}
	var node *core.Record
	err = e.App.RunInTransaction(func(txApp core.App) error {
		collection, err := txApp.FindCollectionByNameOrId("nodes")
		if err != nil {
			return err
		}
		node = core.NewRecord(collection)
		node.Set("node_key", request.NodeKey)
		node.Set("name", request.Name)
		node.Set("role", request.Role)
		node.Set("labels", request.Labels)
		node.Set("enroll_secret_hash", hash)
		node.Set("enroll_status", "active")
		node.Set("online", false)
		if e.Auth != nil {
			node.Set("created_by", e.Auth.Id)
		}
		if err := txApp.Save(node); err != nil {
			return err
		}
		return audit.RecordManagement(
			txApp,
			actorID(e),
			audit.ActionNodeCreate,
			node.Id,
			e.RemoteIP(),
			map[string]any{
				"node_key": node.GetString("node_key"),
				"name":     node.GetString("name"),
				"role":     node.GetString("role"),
			},
		)
	})
	if err != nil {
		return e.JSON(http.StatusBadRequest, errorResponse("invalid_node"))
	}

	return e.JSON(http.StatusCreated, map[string]any{
		"node":          node,
		"enroll_secret": secret,
	})
}

// HandleListNodes lists all center nodes without hidden enrollment hashes.
func (api *API) HandleListNodes(e *core.RequestEvent) error {
	nodes, err := e.App.FindRecordsByFilter("nodes", "", "", 1000, 0)
	if err != nil {
		return e.InternalServerError("Failed to list nodes.", err)
	}
	return e.JSON(http.StatusOK, map[string]any{"items": nodes})
}

// HandleRotateEnroll replaces a node's enrollment secret and disconnects active agents.
func (api *API) HandleRotateEnroll(e *core.RequestEvent) error {
	nodeKey := e.Request.PathValue("node_key")
	secret, hash, err := centercrypto.GenerateEnrollSecret()
	if err != nil {
		return e.InternalServerError("Failed to generate enrollment secret.", err)
	}

	err = e.App.RunInTransaction(func(txApp core.App) error {
		node, err := txApp.FindFirstRecordByData("nodes", "node_key", nodeKey)
		if err != nil {
			return err
		}
		node.Set("enroll_secret_hash", hash)
		node.Set("enroll_status", "active")
		node.Set("online", false)
		if err := txApp.Save(node); err != nil {
			return err
		}
		if err := revokeNodeSessions(txApp, node.Id); err != nil {
			return err
		}
		return audit.RecordManagement(
			txApp, actorID(e), audit.ActionNodeRotate, node.Id, e.RemoteIP(),
			map[string]any{"node_key": nodeKey},
		)
	})
	if err != nil {
		return e.JSON(http.StatusNotFound, errorResponse("node_not_found"))
	}

	api.hub.Kick(nodeKey, "rotated")
	return e.JSON(http.StatusOK, map[string]string{"enroll_secret": secret})
}

// HandleRevokeNode revokes enrollment and disconnects active agents.
func (api *API) HandleRevokeNode(e *core.RequestEvent) error {
	nodeKey := e.Request.PathValue("node_key")
	err := e.App.RunInTransaction(func(txApp core.App) error {
		node, err := txApp.FindFirstRecordByData("nodes", "node_key", nodeKey)
		if err != nil {
			return err
		}
		node.Set("enroll_status", "revoked")
		node.Set("online", false)
		if err := txApp.Save(node); err != nil {
			return err
		}
		if err := revokeNodeSessions(txApp, node.Id); err != nil {
			return err
		}
		return audit.RecordManagement(
			txApp, actorID(e), audit.ActionNodeRevoke, node.Id, e.RemoteIP(),
			map[string]any{"node_key": nodeKey},
		)
	})
	if err != nil {
		return e.JSON(http.StatusNotFound, errorResponse("node_not_found"))
	}

	api.hub.Kick(nodeKey, "revoked")
	return e.JSON(http.StatusOK, map[string]string{"enroll_status": "revoked"})
}

func revokeNodeSessions(app core.App, nodeID string) error {
	now := types.NowDateTime()
	for {
		sessions, err := app.FindRecordsByFilter(
			"agent_sessions",
			"node = {:node} && revoked_at = ''",
			"",
			500,
			0,
			map[string]any{"node": nodeID},
		)
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			return nil
		}
		for _, session := range sessions {
			session.Set("revoked_at", now)
			if err := app.Save(session); err != nil {
				return err
			}
		}
	}
}
