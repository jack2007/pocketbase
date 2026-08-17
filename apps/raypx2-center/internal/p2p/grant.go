package p2p

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Claims is the exact grant payload verified by raypx2.
type Claims struct {
	ClientNodeKeyHash string
	ServerNodeKeyHash string
	SessionID         string
	ConnectionID      string
	Epoch             uint64
	Expiry            uint64
}

func NodeKeyHash(nodeKey string) string {
	sum := sha256.Sum256([]byte(nodeKey))
	return hex.EncodeToString(sum[:])
}

func VerifyKeyFromEnrollSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

func HexVerifyKey(secret string) string {
	return hex.EncodeToString(VerifyKeyFromEnrollSecret(secret))
}

func VerifyKeyFromHex(hexKey string) ([]byte, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, err
	}
	if len(key) != sha256.Size {
		return nil, fmt.Errorf("grant mac key must be %d bytes", sha256.Size)
	}
	return key, nil
}

func CanonicalJSON(claims Claims) (string, error) {
	if claims.ClientNodeKeyHash == "" || claims.ServerNodeKeyHash == "" ||
		claims.SessionID == "" || claims.ConnectionID == "" ||
		claims.Epoch == 0 || claims.Expiry == 0 {
		return "", fmt.Errorf("incomplete grant claims")
	}
	return `{"client_node_key_hash":` + jsonString(claims.ClientNodeKeyHash) +
		`,"server_node_key_hash":` + jsonString(claims.ServerNodeKeyHash) +
		`,"session_id":` + jsonString(claims.SessionID) +
		`,"connection_id":` + jsonString(claims.ConnectionID) +
		`,"epoch":` + strconv.FormatUint(claims.Epoch, 10) +
		`,"expiry":` + strconv.FormatUint(claims.Expiry, 10) +
		`,"max_active_quic":1}`, nil
}

func Mint(claims Claims, key []byte) (string, error) {
	if len(key) != sha256.Size {
		return "", fmt.Errorf("grant verify key must be %d bytes", sha256.Size)
	}
	canonical, err := CanonicalJSON(claims)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString([]byte(canonical)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func LooksLike(raw []byte) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return false
	}
	for _, key := range []string{"session_id", "connection_id", "epoch", "seq"} {
		if _, ok := object[key]; ok {
			return true
		}
	}
	typeField, ok := object["type"]
	if !ok {
		return false
	}
	var frameType string
	if json.Unmarshal(typeField, &frameType) != nil {
		return false
	}
	switch frameType {
	case "invite", "grant", "description", "candidate", "end_of_candidates",
		"p2p_ack", "p2p_resume", "p2p_resume_miss":
		return true
	default:
		return false
	}
}

func SessionIDOf(raw []byte) string {
	var object struct {
		SessionID string `json:"session_id"`
	}
	if json.Unmarshal(raw, &object) != nil {
		return ""
	}
	return strings.TrimSpace(object.SessionID)
}

func jsonString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(encoded)
}
