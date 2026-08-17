package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	centercrypto "github.com/pocketbase/pocketbase/apps/raypx2-center/internal/crypto"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/p2p"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/protocol"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

const (
	heartbeatInterval = 15 * time.Second
	summaryInterval   = 30 * time.Second
	maxFrameBytes     = 1 << 20
	maxLastErrorBytes = 4096
)

var (
	hubMu     sync.RWMutex
	hub       = agenthub.New()
	p2pMu     sync.RWMutex
	p2pBroker *p2p.Broker
)

type wsConn struct {
	conn      *websocket.Conn
	id        string
	sessionID string
	writeMu   sync.Mutex
}

func (c *wsConn) ID() string        { return c.id }
func (c *wsConn) SessionID() string { return c.sessionID }

func (c *wsConn) Send(ctx context.Context, frame protocol.Frame) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return wsjson.Write(ctx, c.conn, frame)
}

func (c *wsConn) SendRaw(ctx context.Context, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.Write(ctx, websocket.MessageText, payload)
}

func (c *wsConn) Close(reason string) error {
	return c.conn.Close(websocket.StatusNormalClosure, reason)
}

// SetHub sets the process-wide agent hub used by HandleWS.
func SetHub(value *agenthub.Hub) {
	hubMu.Lock()
	defer hubMu.Unlock()
	hub = value
}

func currentHub() *agenthub.Hub {
	hubMu.RLock()
	defer hubMu.RUnlock()
	return hub
}

// SetP2PBroker installs the process-wide ICE signaling broker.
func SetP2PBroker(broker *p2p.Broker) {
	p2pMu.Lock()
	defer p2pMu.Unlock()
	p2pBroker = broker
}

func currentP2P() *p2p.Broker {
	p2pMu.RLock()
	defer p2pMu.RUnlock()
	return p2pBroker
}

// LookupSession validates an Authorization header and returns its live session and node.
func LookupSession(app core.App, authorization string) (*core.Record, *core.Record, error) {
	token := bearerToken(authorization)
	if token == "" {
		return nil, nil, errInvalidSession
	}
	sessions, err := app.FindAllRecords("agent_sessions")
	if err != nil {
		return nil, nil, err
	}
	now := types.NowDateTime()
	for _, session := range sessions {
		if !session.GetDateTime("revoked_at").IsZero() ||
			!session.GetDateTime("expires_at").After(now) ||
			!centercrypto.VerifySecret(session.GetString("token_hash"), token) {
			continue
		}
		node, err := app.FindRecordById("nodes", session.GetString("node"))
		if err != nil {
			return nil, nil, err
		}
		if node.GetString("enroll_status") != "active" {
			return nil, nil, errInvalidSession
		}
		return session, node, nil
	}
	return nil, nil, errInvalidSession
}

// RevokeSession marks an agent session as revoked.
func RevokeSession(app core.App, sessionID string) error {
	session, err := app.FindRecordById("agent_sessions", sessionID)
	if err != nil {
		return err
	}
	if session.GetDateTime("revoked_at").IsZero() {
		session.Set("revoked_at", types.NowDateTime())
		return app.Save(session)
	}
	return nil
}

// HandleWS authenticates an agent and serves its JSON protocol connection.
func HandleWS(e *core.RequestEvent) error {
	agentHub := currentHub()
	token := bearerToken(e.Request.Header.Get("Authorization"))
	if token == "" {
		return invalidCredentials(e)
	}
	session, node, err := LookupSession(e.App, e.Request.Header.Get("Authorization"))
	if err != nil {
		return invalidCredentials(e)
	}
	nodeKey := node.GetString("node_key")
	registrationEpoch := agentHub.Epoch(nodeKey)

	conn, err := websocket.Accept(e.Response, e.Request, nil)
	if err != nil {
		return err
	}
	conn.SetReadLimit(maxFrameBytes)
	transport := &wsConn{conn: conn, id: uuid.NewString(), sessionID: session.Id}
	if !registrationStillActive(e.App, session.Id, node.Id) {
		_ = conn.Close(websocket.StatusPolicyViolation, "invalid session")
		return nil
	}
	if _, err := agentHub.RegisterAtEpoch(nodeKey, registrationEpoch, transport); err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "stale registration")
		return nil
	}
	defer func() {
		if agentHub.IsCurrent(nodeKey, transport.ID()) {
			_ = markNodeOffline(e.App, node.Id)
		}
		agentHub.Unregister(nodeKey, transport.ID())
		_ = conn.Close(websocket.StatusNormalClosure, "connection closed")
	}()

	welcome, _ := json.Marshal(map[string]int{
		"heartbeat_interval_sec": int(heartbeatInterval / time.Second),
		"summary_interval_sec":   int(summaryInterval / time.Second),
	})
	if err := transport.Send(e.Request.Context(), protocol.Frame{
		Type:    "welcome",
		TS:      frameTimestamp(),
		Payload: welcome,
	}); err != nil {
		return nil
	}
	if err := touchNode(e.App, node.Id); err != nil {
		e.App.Logger().Error("failed to mark agent online", "node", nodeKey, "error", err)
	}

	for {
		readCtx, cancel := context.WithTimeout(e.Request.Context(), 3*heartbeatInterval)
		_, data, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			return nil
		}
		if err := touchNode(e.App, node.Id); err != nil {
			e.App.Logger().Error("failed to update agent last seen", "node", nodeKey, "error", err)
		}
		if p2p.LooksLike(data) {
			if broker := currentP2P(); broker != nil {
				if fwdErr := broker.Forward(nodeKey, data); fwdErr != nil {
					e.App.Logger().Error("failed to forward p2p frame", "node", nodeKey, "error", fwdErr)
				}
			}
			continue
		}
		var frame protocol.Frame
		if err := json.Unmarshal(data, &frame); err != nil {
			return nil
		}
		switch frame.Type {
		case "ping":
			if err := transport.Send(e.Request.Context(), protocol.Frame{
				Type:    "pong",
				ID:      frame.ID,
				TS:      frameTimestamp(),
				Payload: frame.Payload,
			}); err != nil {
				return nil
			}
		case "status_summary":
			if err := applyStatusSummary(e.App, node, frame.Payload); err != nil {
				e.App.Logger().Error("failed to save agent status", "node", nodeKey, "error", err)
				if sendAgentError(e.Request.Context(), transport, frame, "invalid_status_summary") != nil {
					return nil
				}
			}
		case "config_snapshot":
			if err := applyConfigSnapshot(e.App, node, frame.Payload); err != nil {
				e.App.Logger().Error("failed to save config snapshot", "node", nodeKey, "error", err)
				if sendAgentError(e.Request.Context(), transport, frame, "invalid_config_snapshot") != nil {
					return nil
				}
			}
		case "http_proxy_res":
			agentHub.HandleFrame(nodeKey, frame)
		}
	}
}

func applyStatusSummary(app core.App, node *core.Record, payload json.RawMessage) error {
	var summary map[string]any
	if err := json.Unmarshal(payload, &summary); err != nil {
		return err
	}
	if summary == nil {
		return errors.New("status summary must be an object")
	}
	if path, ok := sensitivePath(summary, nil); ok {
		return fmt.Errorf("status summary contains sensitive path %q", path)
	}
	lastError := truncateUTF8(stringValue(summary["last_error"]), maxLastErrorBytes)
	summary["last_error"] = lastError
	if err := touchNode(app, node.Id); err != nil {
		return err
	}

	status, err := app.FindFirstRecordByData("node_status", "node", node.Id)
	if err != nil {
		collection, collectionErr := app.FindCollectionByNameOrId("node_status")
		if collectionErr != nil {
			return collectionErr
		}
		status = core.NewRecord(collection)
		status.Set("node", node.Id)
	}
	status.Set("health_status", stringValue(summary["health_status"]))
	status.Set("uptime_seconds", intValue(summary["uptime_seconds"]))
	status.Set("last_error", lastError)
	status.Set("config_hash", stringValue(summary["config_hash"]))
	status.Set("summary", summary)
	status.Set("fetched_at", types.NowDateTime())
	return app.Save(status)
}

func applyConfigSnapshot(app core.App, node *core.Record, payload json.RawMessage) error {
	var raw any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return err
	}
	if path, ok := sensitivePath(raw, nil); ok {
		return fmt.Errorf("config snapshot contains sensitive path %q", path)
	}
	var snapshot struct {
		ContentHash string         `json:"content_hash"`
		Content     map[string]any `json:"content"`
	}
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return err
	}
	if snapshot.Content == nil {
		return errors.New("config snapshot content must be an object")
	}
	collection, err := app.FindCollectionByNameOrId("config_revisions")
	if err != nil {
		return err
	}
	revision := core.NewRecord(collection)
	revision.Set("node", node.Id)
	revision.Set("kind", "actual")
	revision.Set("source", "pull")
	revision.Set("content_hash", snapshot.ContentHash)
	revision.Set("content", snapshot.Content)
	return app.Save(revision)
}

func registrationStillActive(app core.App, sessionID, nodeID string) bool {
	session, err := app.FindRecordById("agent_sessions", sessionID)
	if err != nil ||
		!session.GetDateTime("revoked_at").IsZero() ||
		!session.GetDateTime("expires_at").After(types.NowDateTime()) {
		return false
	}
	node, err := app.FindRecordById("nodes", nodeID)
	return err == nil && node.GetString("enroll_status") == "active"
}

func sendAgentError(ctx context.Context, conn *wsConn, request protocol.Frame, code string) error {
	payload, _ := json.Marshal(map[string]string{"code": code})
	return conn.Send(ctx, protocol.Frame{
		Type:    "error",
		ID:      request.ID,
		TS:      frameTimestamp(),
		Payload: payload,
	})
}

func sensitivePath(value any, path []string) (string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			next := appendPath(path, key)
			normalized := strings.ToLower(strings.Join(next, "."))
			keyLower := strings.ToLower(key)
			if isSensitiveKey(keyLower) ||
				normalized == "tls.key" ||
				strings.HasSuffix(normalized, ".tls.key") {
				return strings.Join(next, "."), true
			}
			if found, ok := sensitivePath(child, next); ok {
				return found, true
			}
		}
	case []any:
		for _, child := range typed {
			if found, ok := sensitivePath(child, path); ok {
				return found, true
			}
		}
	}
	return "", false
}

func isSensitiveKey(key string) bool {
	switch key {
	case "token", "enroll_secret", "admin_token", "password", "private_key", "quic_key", "secret":
		return true
	default:
		return false
	}
}

func appendPath(path []string, key string) []string {
	next := make([]string, len(path)+1)
	copy(next, path)
	next[len(path)] = key
	return next
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func touchNode(app core.App, nodeID string) error {
	return app.RunInTransaction(func(txApp core.App) error {
		node, err := txApp.FindRecordById("nodes", nodeID)
		if err != nil {
			return err
		}
		if node.GetString("enroll_status") != "active" {
			return errInvalidSession
		}
		node.Set("online", true)
		node.Set("last_seen_at", types.NowDateTime())
		return txApp.Save(node)
	})
}

func markNodeOffline(app core.App, nodeID string) error {
	return app.RunInTransaction(func(txApp core.App) error {
		node, err := txApp.FindRecordById("nodes", nodeID)
		if err != nil {
			return err
		}
		node.Set("online", false)
		return txApp.Save(node)
	})
}

func frameTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func intValue(value any) int {
	number, ok := value.(float64)
	if !ok {
		return 0
	}
	return int(number)
}
