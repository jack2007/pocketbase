package centerapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/audit"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/configmerge"
	"github.com/pocketbase/pocketbase/core"
)

type putNodeConfigRequest struct {
	Content map[string]any `json:"content"`
}

// HandleGetNodeConfig returns the live, editable, and historical config views.
func (api *API) HandleGetNodeConfig(e *core.RequestEvent) error {
	nodeKey := e.Request.PathValue("node_key")
	node, err := e.App.FindFirstRecordByData("nodes", "node_key", nodeKey)
	if err != nil {
		return e.JSON(http.StatusNotFound, errorResponse("node_not_found"))
	}
	role := node.GetString("role")
	revisions, err := recentConfigRevisions(e.App, node.Id)
	if err != nil {
		return e.InternalServerError("Failed to list config revisions.", err)
	}

	var live any
	editorDraft := map[string]any{}
	if node.GetBool("online") {
		ctx, cancel := context.WithTimeout(e.Request.Context(), proxyTimeout)
		defer cancel()
		body, status, err := api.proxyConfig(ctx, nodeKey, http.MethodGet, configPath(role), nil)
		if err != nil {
			return configProxyError(e, err)
		}
		if status < http.StatusOK || status >= http.StatusMultipleChoices {
			return adminRejected(e, status, body)
		}
		safe, ok := configmerge.Redact(body).(map[string]any)
		if !ok {
			return e.JSON(http.StatusBadGateway, errorResponse("invalid_proxy_response"))
		}
		live = safe
		editorDraft, err = configmerge.EditorDraft(role, safe)
		if err != nil {
			return e.JSON(http.StatusBadGateway, errorResponse("invalid_proxy_response"))
		}
	}
	writablePaths := configmerge.WritablePaths(role)
	if writablePaths == nil {
		writablePaths = []string{}
	}
	return e.JSON(http.StatusOK, map[string]any{
		"node_key":         nodeKey,
		"role":             role,
		"online":           node.GetBool("online"),
		"live":             live,
		"editor_draft":     editorDraft,
		"writable_paths":   writablePaths,
		"recent_revisions": revisions,
	})
}

// HandlePutNodeConfig applies a role-whitelisted single-node config update.
func (api *API) HandlePutNodeConfig(e *core.RequestEvent) error {
	nodeKey := e.Request.PathValue("node_key")
	node, err := e.App.FindFirstRecordByData("nodes", "node_key", nodeKey)
	if err != nil {
		return e.JSON(http.StatusNotFound, errorResponse("node_not_found"))
	}
	role := node.GetString("role")
	if role != "server" && role != "client" {
		return e.JSON(http.StatusBadRequest, errorResponse("unsupported_node_role"))
	}
	if !node.GetBool("online") {
		return e.JSON(http.StatusConflict, errorResponse("node_offline"))
	}

	var request putNodeConfigRequest
	if err := e.BindBody(&request); err != nil {
		return e.JSON(http.StatusBadRequest, errorResponse("invalid_request"))
	}
	if request.Content == nil {
		request.Content = map[string]any{}
	}
	patch, ignored, err := configmerge.TrimForRole(role, request.Content)
	if err != nil {
		if strings.Contains(err.Error(), "templates must not contain TLS private keys or admin tokens") {
			return e.JSON(http.StatusBadRequest, errorResponse("secret_field_forbidden"))
		}
		return e.JSON(http.StatusBadRequest, errorResponse("invalid_config_update"))
	}
	if ignored == nil {
		ignored = []string{}
	}
	if len(patch) == 0 {
		return e.JSON(http.StatusBadRequest, errorResponse("empty_config_update"))
	}

	ctx, cancel := context.WithTimeout(e.Request.Context(), proxyTimeout)
	defer cancel()
	path := configPath(role)
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

	var desired, writeBody map[string]any
	var method string
	switch role {
	case "server":
		desired, err = configmerge.MergeServerConfig(actual, patch)
		writeBody = patch
		method = http.MethodPatch
	case "client":
		desired, err = configmerge.MergeClientPeers(actual, patch)
		writeBody = desired
		method = http.MethodPut
	}
	if err != nil {
		return e.JSON(http.StatusBadRequest, errorResponse("invalid_config_update"))
	}
	adminBody, adminStatus, err := api.proxyConfig(ctx, nodeKey, method, path, writeBody)
	if err != nil {
		return configProxyError(e, err)
	}
	if adminStatus < http.StatusOK || adminStatus >= http.StatusMultipleChoices {
		return adminRejected(e, adminStatus, adminBody)
	}
	revision, err := saveConfigRevision(
		e.App, node, actorID(e), "desired", "manual_edit", desired,
	)
	if err != nil {
		return e.InternalServerError("Failed to save desired config revision.", err)
	}

	liveAfter, status, err := api.proxyConfig(ctx, nodeKey, http.MethodGet, path, nil)
	if err != nil {
		return configProxyError(e, err)
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return adminRejected(e, status, liveAfter)
	}
	safeAfter, ok := configmerge.Redact(liveAfter).(map[string]any)
	if !ok {
		return e.JSON(http.StatusBadGateway, errorResponse("invalid_proxy_response"))
	}
	applied, err := configmerge.EditorDraft(role, safeAfter)
	if err != nil {
		return e.JSON(http.StatusBadGateway, errorResponse("invalid_proxy_response"))
	}
	summary := map[string]any{
		"node_key":       nodeKey,
		"role":           role,
		"content_hash":   revision.GetString("content_hash"),
		"ignored_fields": ignored,
		"admin_status":   adminStatus,
	}
	if err := audit.RecordManagement(
		e.App, actorID(e), audit.ActionNodeConfigUpdate, node.Id, e.RemoteIP(), summary,
	); err != nil {
		return e.InternalServerError("Failed to record config update audit.", err)
	}
	return e.JSON(http.StatusOK, map[string]any{
		"applied":        applied,
		"ignored_fields": ignored,
		"revision_id":    revision.Id,
		"admin_status":   adminStatus,
	})
}

func (api *API) proxyConfig(
	ctx context.Context,
	nodeKey, method, path string,
	body map[string]any,
) (map[string]any, int, error) {
	var bodyB64 string
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		bodyB64 = base64.StdEncoding.EncodeToString(encoded)
	}
	response, err := api.hub.RequestProxy(ctx, nodeKey, agenthub.ProxyRequest{
		Method: method, Path: path, BodyB64: bodyB64,
		Headers:   map[string]string{"Content-Type": "application/json"},
		TimeoutMS: int(proxyTimeout / time.Millisecond),
	})
	if err != nil {
		return nil, 0, err
	}
	if response.Error != "" {
		return nil, 0, errors.New(response.Error)
	}
	if response.Status < 100 || response.Status > 599 {
		return nil, 0, errors.New("invalid_proxy_response")
	}
	decoded, err := base64.StdEncoding.DecodeString(response.BodyB64)
	if err != nil {
		return nil, 0, errors.New("invalid_proxy_response")
	}
	if len(decoded) == 0 {
		return map[string]any{}, response.Status, nil
	}
	var result map[string]any
	if err := json.Unmarshal(decoded, &result); err != nil || result == nil {
		return nil, 0, errors.New("invalid_proxy_response")
	}
	return result, response.Status, nil
}

func configPath(role string) string {
	if role == "server" {
		return "/api/v1/server/config"
	}
	return "/api/v1/config"
}

func configProxyError(e *core.RequestEvent, err error) error {
	switch {
	case errors.Is(err, agenthub.ErrNodeOffline):
		return e.JSON(http.StatusServiceUnavailable, errorResponse("node_offline"))
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return e.JSON(http.StatusGatewayTimeout, errorResponse("tunnel_timeout"))
	case errors.Is(err, agenthub.ErrProxyInflightLimit):
		return e.JSON(http.StatusTooManyRequests, errorResponse("proxy_inflight_limit"))
	case err.Error() == "invalid_proxy_response":
		return e.JSON(http.StatusBadGateway, errorResponse("invalid_proxy_response"))
	default:
		return e.JSON(http.StatusBadGateway, errorResponse("admin_unreachable"))
	}
}

func adminRejected(e *core.RequestEvent, status int, body map[string]any) error {
	safe := configmerge.Redact(body)
	return e.JSON(status, map[string]any{
		"code":         "admin_rejected",
		"message":      "admin_rejected",
		"admin_status": status,
		"admin_body":   safe,
	})
}

func saveConfigRevision(
	app core.App,
	node *core.Record,
	actorID, kind, source string,
	content map[string]any,
) (*core.Record, error) {
	safe, ok := configmerge.Redact(content).(map[string]any)
	if !ok {
		return nil, errors.New("config content must be an object")
	}
	encoded, err := json.Marshal(safe)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(encoded)
	collection, err := app.FindCollectionByNameOrId("config_revisions")
	if err != nil {
		return nil, err
	}
	revision := core.NewRecord(collection)
	revision.Set("node", node.Id)
	revision.Set("kind", kind)
	revision.Set("source", source)
	revision.Set("content_hash", hex.EncodeToString(sum[:]))
	revision.Set("content", safe)
	revision.Set("diff_summary", "single-node config update")
	if actorID != "" {
		revision.Set("actor", actorID)
	}
	if err := app.Save(revision); err != nil {
		return nil, err
	}
	return revision, nil
}

func recentConfigRevisions(app core.App, nodeID string) ([]map[string]any, error) {
	records, err := app.FindRecordsByFilter(
		"config_revisions", "node = {:node}", "-created", 20, 0,
		map[string]any{"node": nodeID},
	)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(records))
	for _, record := range records {
		result = append(result, map[string]any{
			"id":      record.Id,
			"kind":    record.GetString("kind"),
			"source":  record.GetString("source"),
			"created": record.GetDateTime("created"),
		})
	}
	return result, nil
}
