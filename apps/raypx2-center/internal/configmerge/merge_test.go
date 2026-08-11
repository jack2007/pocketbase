package configmerge

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
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

func TestRemoveClientPeerRemovesMatchingPeer(t *testing.T) {
	t.Parallel()
	actual := map[string]any{
		"version": float64(1),
		"peers": []any{
			map[string]any{"peer_id": "peer-a", "quic_peer": "a:443"},
			map[string]any{"peer_id": "peer-b", "quic_peer": "b:443"},
		},
	}
	desired, err := RemoveClientPeer(actual, "peer-a")
	if err != nil {
		t.Fatal(err)
	}
	peers := desired["peers"].([]any)
	if len(peers) != 1 {
		t.Fatalf("peers = %#v", peers)
	}
	if peers[0].(map[string]any)["peer_id"] != "peer-b" {
		t.Fatalf("remaining peer = %#v", peers[0])
	}
	if desired["version"] != float64(1) {
		t.Fatalf("version lost: %#v", desired)
	}
	if len(actual["peers"].([]any)) != 2 {
		t.Fatal("RemoveClientPeer mutated actual")
	}
}

func TestRemoveClientPeerMissingOrEmpty(t *testing.T) {
	t.Parallel()
	actual := map[string]any{
		"peers": []any{map[string]any{"peer_id": "peer-a"}},
	}
	if _, err := RemoveClientPeer(actual, "missing"); !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("missing peer err = %v", err)
	}
	if _, err := RemoveClientPeer(actual, "  "); !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("empty peer err = %v", err)
	}
	if _, err := RemoveClientPeer(map[string]any{}, "peer-a"); !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("no peers err = %v", err)
	}
}

func TestTrimForRoleServerKeepsACLAndCompressionIgnoresRest(t *testing.T) {
	t.Parallel()
	patch, ignored, err := TrimForRole("server", map[string]any{
		"allow_targets": []any{"10.0.0.0/8"},
		"deny_targets":  []any{},
		"connection": map[string]any{
			"compression":        map[string]any{"level": float64(5)},
			"max_send_rate_kbps": float64(100000), // Admin PATCH 当前不接受 → ignored
		},
		"listen": ":443",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := patch["allow_targets"]; !ok {
		t.Fatalf("patch=%#v", patch)
	}
	conn := patch["connection"].(map[string]any)
	if _, ok := conn["compression"]; !ok {
		t.Fatalf("expected compression kept: %#v", conn)
	}
	if _, ok := conn["max_send_rate_kbps"]; ok {
		t.Fatalf("max_send_rate must be ignored for server trim: %#v", conn)
	}
	joined := strings.Join(ignored, ",")
	if !strings.Contains(joined, "listen") || !strings.Contains(joined, "max_send_rate_kbps") {
		t.Fatalf("ignored=%v", ignored)
	}
}

func TestTrimForRoleServerRejectsInvalidCompressionLevel(t *testing.T) {
	t.Parallel()
	for _, level := range []any{float64(0), float64(23), float64(1.5), "3"} {
		level := level
		t.Run(fmt.Sprint(level), func(t *testing.T) {
			t.Parallel()
			_, _, err := TrimForRole("server", map[string]any{
				"connection": map[string]any{
					"compression": map[string]any{"level": level},
				},
			})
			if err == nil || !strings.Contains(err.Error(), "integer between 1 and 22") {
				t.Fatalf("err=%v, want compression level range error", err)
			}
		})
	}
}

func TestMergeServerConfigRejectsInvalidCompressionLevel(t *testing.T) {
	t.Parallel()
	for _, level := range []any{float64(-1), float64(22.5), float64(99), nil} {
		level := level
		t.Run(fmt.Sprint(level), func(t *testing.T) {
			t.Parallel()
			_, err := MergeServerConfig(map[string]any{}, map[string]any{
				"connection": map[string]any{
					"compression": map[string]any{"level": level},
				},
			})
			if err == nil || !strings.Contains(err.Error(), "integer between 1 and 22") {
				t.Fatalf("err=%v, want compression level range error", err)
			}
		})
	}
}

func TestTrimForRoleClientPeersIgnoresStartupOnlyRateFields(t *testing.T) {
	t.Parallel()
	patch, ignored, err := TrimForRole("client", map[string]any{
		"peers": []any{map[string]any{
			"id":         "peer-a",
			"proto_peer": "10.0.0.2:4433",
			"connection": map[string]any{
				"encryption":         "enabled",
				"min_send_rate_kbps": float64(1000),
				"max_send_rate_kbps": float64(50000),
			},
		}},
		"tls": map[string]any{"ca": "certs/ca.crt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	peers := patch["peers"].([]any)
	peer := peers[0].(map[string]any)
	if peer["peer_id"] != "peer-a" || peer["quic_peer"] != "10.0.0.2:4433" {
		t.Fatalf("normalize failed: %#v", peer)
	}
	conn := peer["connection"].(map[string]any)
	if conn["encryption"] != "enabled" {
		t.Fatalf("writable connection field missing: %#v", conn)
	}
	if _, ok := conn["min_send_rate_kbps"]; ok {
		t.Fatalf("startup-only rate must be ignored: %#v", conn)
	}
	joined := strings.Join(ignored, ",")
	for _, want := range []string{"tls", "min_send_rate_kbps", "max_send_rate_kbps"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("ignored=%v want %q", ignored, want)
		}
	}
}

func TestWritablePathsClientExcludesStartupOnlyRateFields(t *testing.T) {
	t.Parallel()
	joined := strings.Join(WritablePaths("client"), ",")
	if strings.Contains(joined, "min_send_rate_kbps") ||
		strings.Contains(joined, "max_send_rate_kbps") {
		t.Fatalf("writable paths contain startup-only rates: %s", joined)
	}
}

func TestEditorDraftServerFlattensDesiredConnection(t *testing.T) {
	t.Parallel()
	draft, err := EditorDraft("server", map[string]any{
		"allow_targets": []any{"10.0.0.0/8"},
		"deny_targets":  []any{},
		"connection_config": map[string]any{
			"desired": map[string]any{
				"compression":        map[string]any{"level": float64(3)},
				"max_send_rate_kbps": float64(0),
			},
			"restart_required": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if draft["allow_targets"] == nil {
		t.Fatal("missing allow_targets")
	}
	conn := draft["connection"].(map[string]any)
	comp := conn["compression"].(map[string]any)
	if comp["level"] != float64(3) {
		t.Fatalf("draft connection=%#v", conn)
	}
	if _, ok := draft["connection_config"]; ok {
		t.Fatal("connection_config must not appear in editor draft")
	}
}

func TestEditorDraftServerCompressionLevelOnly(t *testing.T) {
	t.Parallel()
	draft, err := EditorDraft("server", map[string]any{
		"allow_targets": []any{"10.0.0.0/8"},
		"connection_config": map[string]any{
			"desired": map[string]any{
				"compression": map[string]any{
					"mode":  "zstd",
					"level": float64(5),
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	conn := draft["connection"].(map[string]any)
	comp := conn["compression"].(map[string]any)
	if _, ok := comp["mode"]; ok {
		t.Fatalf("draft must not include compression.mode, got %#v", comp)
	}
	if comp["level"] != float64(5) {
		t.Fatalf("draft compression=%#v", comp)
	}
}

func TestEditorDraftClientProjectsOnlyWritablePeerFields(t *testing.T) {
	t.Parallel()
	draft, err := EditorDraft("client", map[string]any{
		"version": float64(1),
		"peers": []any{map[string]any{
			"id":         "peer-a",
			"proto_peer": "host:443",
			"enabled":    true,
			"status":     "connected",
			"connection": map[string]any{
				"encryption":         "enabled",
				"compression":        map[string]any{"mode": "auto", "level": float64(3), "runtime": true},
				"min_send_rate_kbps": float64(1000),
				"runtime_state":      "ready",
			},
			"port_forwards": []any{map[string]any{
				"listen": ":8080", "target": "127.0.0.1:80", "connections": float64(2),
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"peers": []any{map[string]any{
		"peer_id":   "peer-a",
		"quic_peer": "host:443",
		"enabled":   true,
		"connection": map[string]any{
			"encryption":  "enabled",
			"compression": map[string]any{"mode": "auto", "level": float64(3)},
		},
		"port_forwards": []any{map[string]any{
			"listen": ":8080", "target": "127.0.0.1:80",
		}},
	}}}
	if !reflect.DeepEqual(draft, want) {
		t.Fatalf("draft = %#v, want %#v", draft, want)
	}
}

func TestMergeClientPeersRejectsStartupOnlySendRates(t *testing.T) {
	t.Parallel()
	actual := map[string]any{"peers": []any{
		map[string]any{"peer_id": "peer-a", "quic_peer": "old:443"},
	}}
	_, err := MergeClientPeers(actual, map[string]any{
		"peers": []any{map[string]any{
			"peer_id": "peer-a",
			"connection": map[string]any{
				"min_send_rate_kbps": float64(1000),
				"max_send_rate_kbps": float64(2000),
			},
		}},
	})
	if err == nil {
		t.Fatal("expected startup-only send rates to be rejected")
	}
}

func TestTrimForRoleClientRejectsPeerPassword(t *testing.T) {
	t.Parallel()
	_, _, err := TrimForRole("client", map[string]any{
		"peers": []any{map[string]any{
			"peer_id":  "peer-a",
			"password": "SECRET",
		}},
	})
	if err == nil {
		t.Fatal("expected password field to reject request")
	}
}

func TestTrimForRoleRejectsSecrets(t *testing.T) {
	t.Parallel()
	for name, content := range map[string]map[string]any{
		"tls key": {
			"allow_targets": []any{"10.0.0.0/8"},
			"tls":           map[string]any{"key": "SECRET"},
		},
		"enroll_secret": {
			"peers": []any{map[string]any{
				"peer_id":       "peer-a",
				"enroll_secret": "SECRET",
			}},
		},
		"center enroll file": {
			"center": map[string]any{"enroll_secret_file": "/x"},
		},
	} {
		content := content
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			role := "server"
			if _, ok := content["peers"]; ok {
				role = "client"
			}
			if _, ok := content["center"]; ok {
				role = "client"
				content = map[string]any{
					"peers":  []any{map[string]any{"peer_id": "a"}},
					"center": content["center"],
				}
			}
			_, _, err := TrimForRole(role, content)
			if err == nil {
				t.Fatal("expected secret rejection")
			}
		})
	}
}

func TestMergeClientPeersPartialConnectionPreservesOtherConnectionFields(t *testing.T) {
	t.Parallel()
	actual := map[string]any{"peers": []any{
		map[string]any{
			"peer_id":   "peer-a",
			"quic_peer": "old:443",
			"connection": map[string]any{
				"encryption":         "enabled",
				"compression":        map[string]any{"mode": "disabled", "level": float64(3)},
				"max_send_rate_kbps": float64(0),
			},
		},
	}}
	merged, err := MergeClientPeers(actual, map[string]any{
		"peers": []any{map[string]any{
			"peer_id": "peer-a",
			"connection": map[string]any{
				"compression": map[string]any{"level": float64(5)},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	conn := merged["peers"].([]any)[0].(map[string]any)["connection"].(map[string]any)
	if conn["encryption"] != "enabled" {
		t.Fatalf("encryption wiped: %#v", conn)
	}
	comp := conn["compression"].(map[string]any)
	if comp["mode"] != "disabled" || comp["level"] != float64(5) {
		t.Fatalf("compression wiped: %#v", conn)
	}
	if conn["max_send_rate_kbps"] != float64(0) {
		t.Fatalf("actual startup-only rate was not preserved: %#v", conn)
	}
}

func TestTrimForRoleClientTrimsNonWhitelistPeerFields(t *testing.T) {
	t.Parallel()
	patch, ignored, err := TrimForRole("client", map[string]any{
		"peers": []any{map[string]any{
			"peer_id":   "peer-a",
			"quic_peer": "10.0.0.2:4433",
			"status":    "connected",
			"connection": map[string]any{
				"encryption":         "enabled",
				"compression":        map[string]any{"mode": "auto", "level": float64(3), "unknown": true},
				"min_send_rate_kbps": float64(1000),
				"legacy_field":       "drop",
			},
			"port_forwards": []any{map[string]any{
				"listen": ":8080", "target": "127.0.0.1:80", "extra": "x",
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	peer := patch["peers"].([]any)[0].(map[string]any)
	if _, ok := peer["status"]; ok {
		t.Fatalf("status must be trimmed: %#v", peer)
	}
	conn := peer["connection"].(map[string]any)
	if _, ok := conn["legacy_field"]; ok {
		t.Fatalf("connection legacy_field must be trimmed: %#v", conn)
	}
	if _, ok := conn["min_send_rate_kbps"]; ok {
		t.Fatalf("startup-only rate must be trimmed: %#v", conn)
	}
	comp := conn["compression"].(map[string]any)
	if _, ok := comp["unknown"]; ok {
		t.Fatalf("compression unknown must be trimmed: %#v", comp)
	}
	forward := peer["port_forwards"].([]any)[0].(map[string]any)
	if _, ok := forward["extra"]; ok {
		t.Fatalf("port_forward extra must be trimmed: %#v", forward)
	}
	joined := strings.Join(ignored, ",")
	for _, want := range []string{"status", "min_send_rate_kbps", "legacy_field", "unknown", "extra"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("ignored=%v want %q", ignored, want)
		}
	}
}

func TestTrimForRoleClientKeepsEmptyPortForwards(t *testing.T) {
	t.Parallel()
	patch, _, err := TrimForRole("client", map[string]any{
		"peers": []any{map[string]any{
			"peer_id":       "peer-a",
			"port_forwards": []any{},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	peer := patch["peers"].([]any)[0].(map[string]any)
	forwards, ok := peer["port_forwards"]
	if !ok {
		t.Fatal("port_forwards must be preserved in patch")
	}
	if len(forwards.([]any)) != 0 {
		t.Fatalf("port_forwards=%#v want empty slice", forwards)
	}
}

func TestTrimForRoleClientIgnoresInvalidStartupOnlySendRate(t *testing.T) {
	t.Parallel()
	patch, ignored, err := TrimForRole("client", map[string]any{
		"peers": []any{map[string]any{
			"peer_id": "peer-a",
			"connection": map[string]any{
				"encryption":         "enabled",
				"min_send_rate_kbps": float64(1.5),
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	peer := patch["peers"].([]any)[0].(map[string]any)
	if _, ok := peer["connection"].(map[string]any)["min_send_rate_kbps"]; ok {
		t.Fatalf("startup-only rate must be ignored: %#v", peer)
	}
	if !strings.Contains(strings.Join(ignored, ","), "min_send_rate_kbps") {
		t.Fatalf("ignored=%v", ignored)
	}
}

func TestMergeClientPeersRejectsStartupOnlySendRate(t *testing.T) {
	t.Parallel()
	actual := map[string]any{"peers": []any{}}
	_, err := MergeClientPeers(actual, map[string]any{
		"peers": []any{map[string]any{
			"peer_id": "peer-a",
			"connection": map[string]any{
				"max_send_rate_kbps": float64(1.5),
			},
		}},
	})
	if err == nil {
		t.Fatal("expected fractional max_send_rate_kbps to be rejected")
	}
}

func TestRedactMasksSecrets(t *testing.T) {
	t.Parallel()
	out := Redact(map[string]any{
		"allow_targets": []any{"10.0.0.0/8"},
		"tls":           map[string]any{"key": "SECRET", "ca": "certs/ca.crt"},
		"admin_token":   "SECRET",
	}).(map[string]any)
	if out["admin_token"] != "[REDACTED]" {
		t.Fatalf("%#v", out)
	}
	tls := out["tls"].(map[string]any)
	if tls["key"] != "[REDACTED]" || tls["ca"] != "certs/ca.crt" {
		t.Fatalf("%#v", tls)
	}
}
