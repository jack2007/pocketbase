package p2p

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestNodeKeyHashMatchesSHA256Hex(t *testing.T) {
	sum := sha256.Sum256([]byte("client-node-key"))
	if got := NodeKeyHash("client-node-key"); got != hex.EncodeToString(sum[:]) {
		t.Fatalf("NodeKeyHash = %s", got)
	}
}

func TestMintGrantRoundTripShape(t *testing.T) {
	key := VerifyKeyFromEnrollSecret("enroll-secret")
	grant, err := Mint(Claims{
		ClientNodeKeyHash: NodeKeyHash("client-node-key"),
		ServerNodeKeyHash: NodeKeyHash("server-node-key"),
		SessionID:         "session-A",
		ConnectionID:      "conn-7",
		Epoch:             9,
		Expiry:            2000,
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(grant, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		t.Fatalf("grant shape = %q", grant)
	}
	if strings.ContainsAny(grant, "+/=") {
		t.Fatalf("grant is not base64url: %q", grant)
	}
}

func TestLooksLikeP2PFrame(t *testing.T) {
	if !LooksLike([]byte(`{"id":"1","session_id":"s","connection_id":"c","epoch":1,"seq":1,"type":"invite","payload":{}}`)) {
		t.Fatal("invite should look like p2p")
	}
	if LooksLike([]byte(`{"type":"ping","ts":"t"}`)) {
		t.Fatal("ping should not look like p2p")
	}
}
