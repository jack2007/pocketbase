package centerapi

import (
	"net/http"

	"github.com/google/uuid"
	centercrypto "github.com/pocketbase/pocketbase/apps/raypx2-center/internal/crypto"
	"github.com/pocketbase/pocketbase/core"
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
	collection, err := e.App.FindCollectionByNameOrId("nodes")
	if err != nil {
		return e.InternalServerError("Failed to load nodes.", err)
	}
	node := core.NewRecord(collection)
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
	if err := e.App.Save(node); err != nil {
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
