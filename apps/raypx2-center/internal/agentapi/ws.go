package agentapi

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	centercrypto "github.com/pocketbase/pocketbase/apps/raypx2-center/internal/crypto"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/protocol"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

const (
	heartbeatInterval = 15 * time.Second
	summaryInterval   = 30 * time.Second
	maxFrameBytes     = 1 << 20
)

var (
	hubMu sync.RWMutex
	hub   = agenthub.New()
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
	session, node, err := LookupSession(e.App, e.Request.Header.Get("Authorization"))
	if err != nil {
		return invalidCredentials(e)
	}

	conn, err := websocket.Accept(e.Response, e.Request, nil)
	if err != nil {
		return err
	}
	conn.SetReadLimit(maxFrameBytes)
	transport := &wsConn{conn: conn, id: uuid.NewString(), sessionID: session.Id}
	agentHub := currentHub()
	nodeKey := node.GetString("node_key")
	agentHub.Register(nodeKey, transport)
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
		var frame protocol.Frame
		err := wsjson.Read(readCtx, conn, &frame)
		cancel()
		if err != nil {
			return nil
		}
		if err := touchNode(e.App, node.Id); err != nil {
			e.App.Logger().Error("failed to update agent last seen", "node", nodeKey, "error", err)
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
			}
		case "config_snapshot":
			if err := applyConfigSnapshot(e.App, node, frame.Payload); err != nil {
				e.App.Logger().Error("failed to save config snapshot", "node", nodeKey, "error", err)
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
	status.Set("last_error", stringValue(summary["last_error"]))
	status.Set("config_hash", stringValue(summary["config_hash"]))
	status.Set("summary", summary)
	status.Set("fetched_at", types.NowDateTime())
	return app.Save(status)
}

func applyConfigSnapshot(app core.App, node *core.Record, payload json.RawMessage) error {
	var snapshot struct {
		ContentHash string         `json:"content_hash"`
		Content     map[string]any `json:"content"`
	}
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return err
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

func touchNode(app core.App, nodeID string) error {
	node, err := app.FindRecordById("nodes", nodeID)
	if err != nil {
		return err
	}
	node.Set("online", true)
	node.Set("last_seen_at", types.NowDateTime())
	return app.Save(node)
}

func markNodeOffline(app core.App, nodeID string) error {
	node, err := app.FindRecordById("nodes", nodeID)
	if err != nil {
		return err
	}
	node.Set("online", false)
	return app.Save(node)
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
