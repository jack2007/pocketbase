package configmerge

import (
	"reflect"
	"testing"
)

func TestMergeRejectsTLSKeyAndAdminTokens(t *testing.T) {
	t.Parallel()
	actual := map[string]any{"version": float64(1), "peers": []any{}}

	for name, template := range map[string]map[string]any{
		"tls key": {
			"peers": []any{map[string]any{
				"peer_id": "peer-a",
				"tls":     map[string]any{"key": "SECRET"},
			}},
		},
		"admin token": {
			"peers": []any{map[string]any{
				"peer_id":     "peer-a",
				"admin_token": "SECRET",
			}},
		},
	} {
		template := template
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := MergeClientPeers(actual, template); err == nil {
				t.Fatal("expected secret field to be rejected")
			}
		})
	}
}

func TestMergeServerACLOnlyAllowsWhitelistedFields(t *testing.T) {
	t.Parallel()
	actual := map[string]any{
		"role":          "server",
		"allow_targets": []any{"10.0.0.0/8"},
		"deny_targets":  []any{},
	}

	merged, err := MergeServerACL(actual, map[string]any{
		"allow_targets": []any{"127.0.0.0/8"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(merged["allow_targets"], []any{"127.0.0.0/8"}) {
		t.Fatalf("allow_targets = %#v", merged["allow_targets"])
	}
	if merged["role"] != "server" {
		t.Fatalf("role changed: %#v", merged)
	}
	if _, err := MergeServerACL(actual, map[string]any{"tls": map[string]any{"key": "SECRET"}}); err == nil {
		t.Fatal("expected tls key to be rejected")
	}
	if _, err := MergeServerACL(actual, map[string]any{"listen": ":443"}); err == nil {
		t.Fatal("expected non-whitelisted server field to be rejected")
	}
}

func TestMergeClientPeersUpsertsByPeerID(t *testing.T) {
	t.Parallel()
	actual := map[string]any{
		"version": float64(1),
		"peers": []any{
			map[string]any{"peer_id": "peer-a", "quic_peer": "old:443", "enabled": true},
			map[string]any{"peer_id": "peer-b", "quic_peer": "keep:443"},
		},
	}
	template := map[string]any{
		"peers": []any{
			map[string]any{"peer_id": "peer-a", "enabled": false},
			map[string]any{"peer_id": "peer-c", "quic_peer": "new:443"},
		},
	}

	merged, err := MergeClientPeers(actual, template)
	if err != nil {
		t.Fatal(err)
	}
	peers := merged["peers"].([]any)
	if len(peers) != 3 {
		t.Fatalf("peers = %#v", peers)
	}
	first := peers[0].(map[string]any)
	if first["quic_peer"] != "old:443" || first["enabled"] != false {
		t.Fatalf("peer-a = %#v", first)
	}
	if peers[1].(map[string]any)["peer_id"] != "peer-b" || peers[2].(map[string]any)["peer_id"] != "peer-c" {
		t.Fatalf("peer order/upsert = %#v", peers)
	}
}

func TestMergeClientPeersRejectsUnknownAndMissingPeerID(t *testing.T) {
	t.Parallel()
	actual := map[string]any{"version": float64(1), "peers": []any{}}
	for _, template := range []map[string]any{
		{"unknown": true},
		{"peers": []any{map[string]any{"quic_peer": "host:443"}}},
		{"peers": []any{map[string]any{"peer_id": "a", "password": "SECRET"}}},
	} {
		if _, err := MergeClientPeers(actual, template); err == nil {
			t.Fatalf("expected rejection for %#v", template)
		}
	}
}
