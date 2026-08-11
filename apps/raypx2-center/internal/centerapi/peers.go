package centerapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/audit"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/configmerge"
	"github.com/pocketbase/pocketbase/core"
)

// HandleDeleteNodePeer removes one client peer via Admin full-config PUT.
func (api *API) HandleDeleteNodePeer(e *core.RequestEvent) error {
	nodeKey := e.Request.PathValue("node_key")
	peerID := strings.TrimSpace(e.Request.PathValue("peer_id"))
	if peerID == "" {
		return e.JSON(http.StatusNotFound, errorResponse("peer_not_found"))
	}

	node, err := e.App.FindFirstRecordByData("nodes", "node_key", nodeKey)
	if err != nil {
		return e.JSON(http.StatusNotFound, errorResponse("node_not_found"))
	}
	role := node.GetString("role")
	if role != "client" {
		return e.JSON(http.StatusBadRequest, errorResponse("unsupported_node_role"))
	}
	if !node.GetBool("online") {
		return e.JSON(http.StatusConflict, errorResponse("node_offline"))
	}

	ctx, cancel := context.WithTimeout(e.Request.Context(), proxyTimeout)
	defer cancel()
	path := configPath("client")
	actual, status, err := api.proxyConfig(ctx, nodeKey, http.MethodGet, path, nil)
	if err != nil {
		return configProxyError(e, err)
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return adminRejected(e, status, actual)
	}
	if _, err := saveConfigRevision(e.App, node, actorID(e), "actual", "pull", actual); err != nil {
		return e.InternalServerError("Failed to save actual config revision.", err)
	}

	normalized, err := configmerge.NormalizeClientPeers(actual)
	if err != nil {
		return e.JSON(http.StatusBadGateway, errorResponse("invalid_proxy_response"))
	}
	desired, err := configmerge.RemoveClientPeer(normalized, peerID)
	if err != nil {
		if errors.Is(err, configmerge.ErrPeerNotFound) {
			return e.JSON(http.StatusNotFound, errorResponse("peer_not_found"))
		}
		return e.JSON(http.StatusBadRequest, errorResponse("invalid_config_update"))
	}

	adminBody, adminStatus, err := api.proxyConfig(ctx, nodeKey, http.MethodPut, path, desired)
	if err != nil {
		return configProxyError(e, err)
	}
	if adminStatus < http.StatusOK || adminStatus >= http.StatusMultipleChoices {
		return adminRejected(e, adminStatus, adminBody)
	}

	revision, err := saveConfigRevision(
		e.App, node, actorID(e), "desired", "peer_delete", desired,
	)
	if err != nil {
		return e.InternalServerError("Failed to save desired config revision.", err)
	}
	summary := map[string]any{
		"node_key":     nodeKey,
		"peer_id":      peerID,
		"content_hash": revision.GetString("content_hash"),
		"admin_status": adminStatus,
	}
	if err := audit.RecordManagement(
		e.App, actorID(e), audit.ActionNodePeerDelete, node.Id, e.RemoteIP(), summary,
	); err != nil {
		return e.InternalServerError("Failed to record peer delete audit.", err)
	}

	return e.JSON(http.StatusOK, map[string]any{
		"peer_id":      peerID,
		"revision_id":  revision.Id,
		"admin_status": adminStatus,
	})
}
