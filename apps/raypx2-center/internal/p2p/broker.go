package p2p

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/turncred"
)

const (
	DefaultSTUNURL = "stun:64.176.42.49:8744"
	DefaultTURNURL = "turn:64.176.42.49:8744?transport=udp"
	DefaultTTL     = int64(86400)
)

type Config struct {
	STUNURLs         []string
	TURNURLs         []string
	SharedSecretFile string
	CredentialTTL    int64
}

func ConfigFromEnv() Config {
	stun := splitCSV(os.Getenv("RAYPX2_STUN_URLS"))
	if len(stun) == 0 {
		stun = []string{DefaultSTUNURL}
	}
	turn := splitCSV(os.Getenv("RAYPX2_TURN_URLS"))
	if len(turn) == 0 {
		turn = []string{DefaultTURNURL}
	}
	secret := os.Getenv("RAYPX2_TURN_SECRET_FILE")
	if secret == "" {
		secret = "/etc/raypx2/coturn-rest-secret"
	}
	return Config{
		STUNURLs:         stun,
		TURNURLs:         turn,
		SharedSecretFile: secret,
		CredentialTTL:    DefaultTTL,
	}
}

type Session struct {
	SessionID     string
	ConnectionID  string
	Epoch         uint64
	ClientNodeKey string
	ServerNodeKey string
}

type Broker struct {
	hub    *agenthub.Hub
	cfg    Config
	secret []byte
	turnOn bool

	mu       sync.Mutex
	sessions map[string]Session
}

func NewBroker(hub *agenthub.Hub, cfg Config) *Broker {
	if cfg.CredentialTTL <= 0 {
		cfg.CredentialTTL = DefaultTTL
	}
	if len(cfg.STUNURLs) == 0 {
		cfg.STUNURLs = []string{DefaultSTUNURL}
	}
	if len(cfg.TURNURLs) == 0 {
		cfg.TURNURLs = []string{DefaultTURNURL}
	}
	enabled, secret, _, _ := turncred.EvaluateTURN(turncred.Config{
		STUNURLs:         cfg.STUNURLs,
		TURNURLs:         cfg.TURNURLs,
		SharedSecretFile: cfg.SharedSecretFile,
	})
	return &Broker{
		hub:      hub,
		cfg:      cfg,
		secret:   secret,
		turnOn:   enabled,
		sessions: make(map[string]Session),
	}
}

type CreateRequest struct {
	ClientNodeKey string
	ServerNodeKey string
	ConnectionID  string
	GrantMACKey   []byte
	Now           time.Time
}

func (b *Broker) Create(req CreateRequest) (Session, error) {
	if req.ClientNodeKey == "" || req.ServerNodeKey == "" {
		return Session{}, errors.New("client_node_key and server_node_key are required")
	}
	if req.ClientNodeKey == req.ServerNodeKey {
		return Session{}, errors.New("client and server must be different nodes")
	}
	if !b.hub.HasConnection(req.ClientNodeKey) || !b.hub.HasConnection(req.ServerNodeKey) {
		return Session{}, errors.New("both agents must be online")
	}
	if len(req.GrantMACKey) != 32 {
		return Session{}, errors.New("server grant mac key is missing")
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	connectionID := req.ConnectionID
	if connectionID == "" {
		connectionID = "conn-0"
	}
	session := Session{
		SessionID:     uuid.NewString(),
		ConnectionID:  connectionID,
		Epoch:         1,
		ClientNodeKey: req.ClientNodeKey,
		ServerNodeKey: req.ServerNodeKey,
	}
	expiry := uint64(now.Unix() + b.cfg.CredentialTTL)
	grant, err := Mint(Claims{
		ClientNodeKeyHash: NodeKeyHash(req.ClientNodeKey),
		ServerNodeKeyHash: NodeKeyHash(req.ServerNodeKey),
		SessionID:         session.SessionID,
		ConnectionID:      session.ConnectionID,
		Epoch:             session.Epoch,
		Expiry:            expiry,
	}, req.GrantMACKey)
	if err != nil {
		return Session{}, err
	}
	payload := map[string]any{
		"grant":     grant,
		"stun_urls": b.cfg.STUNURLs,
	}
	if b.turnOn && len(b.secret) > 0 {
		username, password := turncred.IssueREST(
			b.secret, session.SessionID, session.ConnectionID, session.Epoch,
			now.Unix(), b.cfg.CredentialTTL,
		)
		payload["turn"] = map[string]any{
			"urls":       b.cfg.TURNURLs,
			"username":   username,
			"password":   password,
			"expires_at": expiry,
		}
	}
	wire, err := encodeInvite(session, payload)
	if err != nil {
		return Session{}, err
	}
	b.mu.Lock()
	b.sessions[session.SessionID] = session
	b.mu.Unlock()
	if err := b.hub.SendRaw(req.ClientNodeKey, wire); err != nil {
		return Session{}, err
	}
	if err := b.hub.SendRaw(req.ServerNodeKey, wire); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (b *Broker) Forward(fromNodeKey string, raw []byte) error {
	sessionID := SessionIDOf(raw)
	if sessionID == "" {
		return errors.New("p2p frame missing session_id")
	}
	b.mu.Lock()
	session, ok := b.sessions[sessionID]
	b.mu.Unlock()
	if !ok {
		return errors.New("unknown p2p session")
	}
	var peer string
	switch fromNodeKey {
	case session.ClientNodeKey:
		peer = session.ServerNodeKey
	case session.ServerNodeKey:
		peer = session.ClientNodeKey
	default:
		return errors.New("sender is not a session member")
	}
	return b.hub.SendRaw(peer, raw)
}

func encodeInvite(session Session, payload map[string]any) ([]byte, error) {
	frame := map[string]any{
		"id":            uuid.NewString(),
		"session_id":    session.SessionID,
		"connection_id": session.ConnectionID,
		"epoch":         session.Epoch,
		"seq":           uint64(1),
		"type":          "invite",
		"payload":       payload,
	}
	return json.Marshal(frame)
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
