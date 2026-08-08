package agentapi

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/audit"
	centercrypto "github.com/pocketbase/pocketbase/apps/raypx2-center/internal/crypto"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

const sessionTTL = 30 * time.Minute

var errInvalidSession = errors.New("invalid session")

type enrollRequest struct {
	NodeKey      string `json:"node_key"`
	EnrollSecret string `json:"enroll_secret"`
	Hostname     string `json:"hostname"`
	Version      string `json:"version"`
	Role         string `json:"role"`
}

type sessionResponse struct {
	Token     string         `json:"token"`
	ExpiresAt types.DateTime `json:"expires_at"`
}

// HandleEnroll validates a node enrollment secret and creates an agent session.
func HandleEnroll(e *core.RequestEvent) error {
	var request enrollRequest
	if err := e.BindBody(&request); err != nil {
		return rejectEnroll(e, "")
	}

	node, err := e.App.FindFirstRecordByData("nodes", "node_key", request.NodeKey)
	if err != nil ||
		node.GetString("enroll_status") != "active" ||
		!centercrypto.VerifySecret(node.GetString("enroll_secret_hash"), request.EnrollSecret) {
		nodeID := ""
		if node != nil {
			nodeID = node.Id
		}
		return rejectEnroll(e, nodeID)
	}

	token, err := generateToken()
	if err != nil {
		return rejectEnroll(e, node.Id)
	}
	tokenHash, err := centercrypto.HashToken(token)
	if err != nil {
		return rejectEnroll(e, node.Id)
	}
	expiresAt := types.NowDateTime().Add(sessionTTL)

	err = e.App.RunInTransaction(func(txApp core.App) error {
		txNode, err := txApp.FindRecordById("nodes", node.Id)
		if err != nil {
			return err
		}
		if txNode.GetString("enroll_status") != "active" ||
			!centercrypto.VerifySecret(txNode.GetString("enroll_secret_hash"), request.EnrollSecret) {
			return errInvalidSession
		}

		sessions, err := txApp.FindCollectionByNameOrId("agent_sessions")
		if err != nil {
			return err
		}
		session := core.NewRecord(sessions)
		session.Set("node", txNode.Id)
		session.Set("token_hash", tokenHash)
		session.Set("expires_at", expiresAt)
		session.Set("client_info", map[string]any{
			"hostname": request.Hostname,
			"version":  request.Version,
			"role":     request.Role,
		})
		if err := txApp.Save(session); err != nil {
			return err
		}

		txNode.Set("hostname", request.Hostname)
		txNode.Set("agent_version", request.Version)
		if request.Role != "" {
			txNode.Set("role", request.Role)
		}
		if err := txApp.Save(txNode); err != nil {
			return err
		}
		return audit.RecordAgentEnroll(txApp, txNode.Id, e.RemoteIP(), true)
	})
	if errors.Is(err, errInvalidSession) {
		return rejectEnroll(e, node.Id)
	}
	if err != nil {
		e.App.Logger().Error("agent enrollment failed", "error", err)
		return rejectEnroll(e, node.Id)
	}

	return e.JSON(http.StatusOK, sessionResponse{Token: token, ExpiresAt: expiresAt})
}

// HandleRefresh validates and rotates a live bearer session token.
func HandleRefresh(e *core.RequestEvent) error {
	token := bearerToken(e.Request.Header.Get("Authorization"))
	if token == "" {
		return invalidCredentials(e)
	}

	var response sessionResponse
	err := e.App.RunInTransaction(func(txApp core.App) error {
		sessions, err := txApp.FindAllRecords("agent_sessions")
		if err != nil {
			return err
		}

		var matched *core.Record
		now := types.NowDateTime()
		for _, session := range sessions {
			if !session.GetDateTime("revoked_at").IsZero() ||
				!session.GetDateTime("expires_at").After(now) {
				continue
			}
			if centercrypto.VerifySecret(session.GetString("token_hash"), token) {
				matched = session
				break
			}
		}
		if matched == nil {
			return errInvalidSession
		}

		newToken, err := generateToken()
		if err != nil {
			return err
		}
		newHash, err := centercrypto.HashToken(newToken)
		if err != nil {
			return err
		}
		expiresAt := types.NowDateTime().Add(sessionTTL)
		matched.Set("token_hash", newHash)
		matched.Set("expires_at", expiresAt)
		if err := txApp.Save(matched); err != nil {
			return err
		}
		response = sessionResponse{Token: newToken, ExpiresAt: expiresAt}
		return nil
	})
	if errors.Is(err, errInvalidSession) {
		return invalidCredentials(e)
	}
	if err != nil {
		e.App.Logger().Error("agent session refresh failed", "error", err)
		return invalidCredentials(e)
	}
	return e.JSON(http.StatusOK, response)
}

func rejectEnroll(e *core.RequestEvent, nodeID string) error {
	if err := audit.RecordAgentEnroll(e.App, nodeID, e.RemoteIP(), false); err != nil {
		e.App.Logger().Error("failed to record agent enrollment audit", "error", err)
	}
	return invalidCredentials(e)
}

func invalidCredentials(e *core.RequestEvent) error {
	return e.JSON(http.StatusUnauthorized, map[string]string{"message": "invalid credentials"})
}

func bearerToken(header string) string {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return ""
	}
	return token
}

func generateToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
