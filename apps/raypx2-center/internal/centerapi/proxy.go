package centerapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/audit"
	"github.com/pocketbase/pocketbase/core"
)

const proxyTimeout = 10 * time.Second

type proxyRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

// HandleProxy forwards a superuser request to a connected node's Admin API.
func (api *API) HandleProxy(e *core.RequestEvent) error {
	started := time.Now()
	nodeKey := e.Request.PathValue("node_key")
	node, err := e.App.FindFirstRecordByData("nodes", "node_key", nodeKey)
	if err != nil {
		return e.JSON(http.StatusNotFound, errorResponse("node_not_found"))
	}

	var request proxyRequest
	status := http.StatusBadRequest
	defer func() {
		actorID := ""
		if e.Auth != nil {
			actorID = e.Auth.Id
		}
		if err := audit.RecordProxyRequest(
			e.App,
			actorID,
			node.Id,
			e.RemoteIP(),
			request.Method,
			request.Path,
			status,
			time.Since(started),
		); err != nil {
			e.App.Logger().Error("failed to record proxy request audit", "error", err)
		}
	}()

	if err := e.BindBody(&request); err != nil {
		return e.JSON(status, errorResponse("invalid_request"))
	}
	request.Method = strings.ToUpper(strings.TrimSpace(request.Method))
	if request.Method == "" {
		request.Method = http.MethodGet
	}
	if !strings.HasPrefix(request.Path, "/api/v1/") {
		return e.JSON(status, errorResponse("invalid_proxy_path"))
	}

	ctx, cancel := context.WithTimeout(e.Request.Context(), proxyTimeout)
	defer cancel()
	response, err := api.hub.RequestProxy(ctx, nodeKey, agenthub.ProxyRequest{
		Method:    request.Method,
		Path:      request.Path,
		Headers:   request.Headers,
		BodyB64:   base64.StdEncoding.EncodeToString(request.Body),
		TimeoutMS: int(proxyTimeout / time.Millisecond),
	})
	if err != nil {
		switch {
		case errors.Is(err, agenthub.ErrNodeOffline):
			status = http.StatusServiceUnavailable
			return e.JSON(status, errorResponse("node_offline"))
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			status = http.StatusGatewayTimeout
			return e.JSON(status, errorResponse("tunnel_timeout"))
		case errors.Is(err, agenthub.ErrProxyInflightLimit):
			status = http.StatusTooManyRequests
			return e.JSON(status, errorResponse("proxy_inflight_limit"))
		default:
			status = http.StatusBadGateway
			return e.JSON(status, errorResponse("admin_unreachable"))
		}
	}
	if response.Error != "" {
		status = http.StatusBadGateway
		return e.JSON(status, errorResponse("admin_unreachable"))
	}
	if response.Status < 100 || response.Status > 599 {
		status = http.StatusBadGateway
		return e.JSON(status, errorResponse("invalid_proxy_response"))
	}
	body, err := base64.StdEncoding.DecodeString(response.BodyB64)
	if err != nil {
		status = http.StatusBadGateway
		return e.JSON(status, errorResponse("invalid_proxy_response"))
	}

	status = response.Status
	for name, value := range response.Headers {
		e.Response.Header().Set(name, value)
	}
	e.Response.WriteHeader(status)
	_, err = e.Response.Write(body)
	return err
}

func errorResponse(code string) map[string]string {
	return map[string]string{"code": code, "message": code}
}
