// Package configmerge applies role-specific, secret-safe configuration templates.
package configmerge

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var errSecretField = errors.New("templates must not contain TLS private keys or admin tokens")

var allowedPeerFields = map[string]bool{
	"peer_id": true, "client_name": true, "quic_peer": true,
	"socks_listen": true, "http_listen": true, "port_forwards": true,
	"quic_connections": true, "quic_reconnect_interval_ms": true,
	"connection": true, "enabled": true,
}

// MergeServerACL overlays only the server Admin API's mutable ACL fields.
func MergeServerACL(actual, templateBody map[string]any) (map[string]any, error) {
	if err := rejectSecrets(templateBody); err != nil {
		return nil, err
	}
	for key, value := range templateBody {
		if key != "allow_targets" && key != "deny_targets" {
			return nil, fmt.Errorf("server template field %q is not allowed", key)
		}
		if err := stringArray(key, value); err != nil {
			return nil, err
		}
	}
	merged, err := cloneMap(actual)
	if err != nil {
		return nil, err
	}
	for key, value := range templateBody {
		merged[key] = value
	}
	return merged, nil
}

// MergeClientPeers upserts whitelisted peer fields by peer_id and preserves
// actual peers omitted from the template.
func MergeClientPeers(actual, templateBody map[string]any) (map[string]any, error) {
	if err := rejectSecrets(templateBody); err != nil {
		return nil, err
	}
	for key := range templateBody {
		if key != "peers" {
			return nil, fmt.Errorf("client template field %q is not allowed", key)
		}
	}
	rawTemplatePeers, ok := templateBody["peers"]
	if !ok {
		return cloneMap(actual)
	}
	templatePeers, ok := rawTemplatePeers.([]any)
	if !ok {
		return nil, errors.New("peers must be an array")
	}
	merged, err := cloneMap(actual)
	if err != nil {
		return nil, err
	}
	actualPeers, _ := merged["peers"].([]any)
	index := make(map[string]int, len(actualPeers))
	for i, raw := range actualPeers {
		if peer, ok := raw.(map[string]any); ok {
			if id, _ := peer["peer_id"].(string); id != "" {
				index[id] = i
			}
		}
	}
	for _, raw := range templatePeers {
		patch, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("each peer must be an object")
		}
		id, _ := patch["peer_id"].(string)
		if strings.TrimSpace(id) == "" {
			return nil, errors.New("each peer requires peer_id")
		}
		if err := validatePeer(patch); err != nil {
			return nil, fmt.Errorf("peer %q: %w", id, err)
		}
		if position, exists := index[id]; exists {
			peer, ok := actualPeers[position].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("actual peer %q is not an object", id)
			}
			for key, value := range patch {
				peer[key] = value
			}
		} else {
			copy, err := cloneMap(patch)
			if err != nil {
				return nil, err
			}
			index[id] = len(actualPeers)
			actualPeers = append(actualPeers, copy)
		}
	}
	merged["peers"] = actualPeers
	return merged, nil
}

func validatePeer(peer map[string]any) error {
	for key, value := range peer {
		if !allowedPeerFields[key] {
			return fmt.Errorf("field %q is not allowed", key)
		}
		switch key {
		case "connection":
			connection, ok := value.(map[string]any)
			if !ok {
				return errors.New("connection must be an object")
			}
			for connectionKey, connectionValue := range connection {
				if connectionKey != "encryption" && connectionKey != "compression" {
					return fmt.Errorf("connection field %q is not allowed", connectionKey)
				}
				if connectionKey == "compression" {
					compression, ok := connectionValue.(map[string]any)
					if !ok {
						return errors.New("connection.compression must be an object")
					}
					for compressionKey := range compression {
						if compressionKey != "mode" && compressionKey != "level" {
							return fmt.Errorf("connection.compression field %q is not allowed", compressionKey)
						}
					}
				}
			}
		case "port_forwards":
			forwards, ok := value.([]any)
			if !ok {
				return errors.New("port_forwards must be an array")
			}
			for _, raw := range forwards {
				forward, ok := raw.(map[string]any)
				if !ok {
					return errors.New("each port_forward must be an object")
				}
				for forwardKey := range forward {
					if forwardKey != "listen" && forwardKey != "target" {
						return fmt.Errorf("port_forward field %q is not allowed", forwardKey)
					}
				}
			}
		}
	}
	return nil
}

func rejectSecrets(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
			if normalized == "admin_token" || normalized == "token" ||
				normalized == "private_key" || normalized == "tls_key" ||
				(normalized == "key" && strings.Contains(strings.ToLower(fmt.Sprint(value)), "tls")) {
				return fmt.Errorf("%w: %s", errSecretField, key)
			}
			if strings.EqualFold(key, "tls") {
				if object, ok := child.(map[string]any); ok {
					if _, exists := object["key"]; exists {
						return fmt.Errorf("%w: tls.key", errSecretField)
					}
				}
			}
			if err := rejectSecrets(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := rejectSecrets(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func stringArray(name string, value any) error {
	items, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s must be an array", name)
	}
	for _, item := range items {
		if _, ok := item.(string); !ok {
			return fmt.Errorf("%s must contain only strings", name)
		}
	}
	return nil
}

func cloneMap(value map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var cloned map[string]any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}
