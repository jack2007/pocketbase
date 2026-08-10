// Package configmerge applies role-specific, secret-safe configuration templates.
package configmerge

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

var errSecretField = errors.New("templates must not contain TLS private keys or admin tokens")

var allowedPeerFields = map[string]bool{
	"peer_id": true, "client_name": true, "quic_peer": true,
	"socks_listen": true, "http_listen": true, "port_forwards": true,
	"quic_connections": true, "quic_reconnect_interval_ms": true,
	"connection": true, "enabled": true,
}

var serverWritablePaths = []string{
	"allow_targets",
	"deny_targets",
	"connection.compression.level",
}

var clientWritablePaths = []string{
	"peers",
	"peers[].peer_id",
	"peers[].client_name",
	"peers[].quic_peer",
	"peers[].socks_listen",
	"peers[].http_listen",
	"peers[].port_forwards",
	"peers[].quic_connections",
	"peers[].quic_reconnect_interval_ms",
	"peers[].enabled",
	"peers[].connection.encryption",
	"peers[].connection.compression.mode",
	"peers[].connection.compression.level",
}

// WritablePaths returns the stable list of editable config paths for a role.
func WritablePaths(role string) []string {
	switch role {
	case "server":
		return append([]string(nil), serverWritablePaths...)
	case "client":
		return append([]string(nil), clientWritablePaths...)
	default:
		return nil
	}
}

// Redact masks secret-bearing values for display, aligned with apply redact rules.
func Redact(value any) any {
	return redactValue(value, "")
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

// MergeServerConfig overlays server ACL and connection.compression.level fields.
func MergeServerConfig(actual, templateBody map[string]any) (map[string]any, error) {
	if err := rejectSecrets(templateBody); err != nil {
		return nil, err
	}
	for key, value := range templateBody {
		switch key {
		case "allow_targets", "deny_targets":
			if err := stringArray(key, value); err != nil {
				return nil, err
			}
		case "connection":
			connection, ok := value.(map[string]any)
			if !ok {
				return nil, errors.New("connection must be an object")
			}
			for connectionKey, connectionValue := range connection {
				if connectionKey != "compression" {
					return nil, fmt.Errorf("server template connection field %q is not allowed", connectionKey)
				}
				compression, ok := connectionValue.(map[string]any)
				if !ok {
					return nil, errors.New("connection.compression must be an object")
				}
				for compressionKey, compressionValue := range compression {
					if compressionKey != "level" {
						return nil, fmt.Errorf("server template connection.compression field %q is not allowed", compressionKey)
					}
					if err := validateCompressionLevel(compressionValue); err != nil {
						return nil, err
					}
				}
			}
		default:
			return nil, fmt.Errorf("server template field %q is not allowed", key)
		}
	}
	merged, err := cloneMap(actual)
	if err != nil {
		return nil, err
	}
	for key, value := range templateBody {
		switch key {
		case "allow_targets", "deny_targets":
			merged[key] = value
		case "connection":
			patchConn := value.(map[string]any)
			actualConn, _ := merged["connection"].(map[string]any)
			if actualConn == nil {
				actualConn = map[string]any{}
			}
			merged["connection"] = deepMergeMap(actualConn, patchConn)
		}
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
				if key == "connection" {
					if patchConn, ok := value.(map[string]any); ok {
						actualConn, _ := peer["connection"].(map[string]any)
						if actualConn == nil {
							actualConn = map[string]any{}
						}
						peer["connection"] = deepMergeMap(actualConn, patchConn)
						continue
					}
				}
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

// NormalizeClientPeers maps legacy peer field names to canonical Admin JSON keys.
func NormalizeClientPeers(content map[string]any) (map[string]any, error) {
	normalized, err := cloneMap(content)
	if err != nil {
		return nil, err
	}
	rawPeers, ok := normalized["peers"]
	if !ok {
		return normalized, nil
	}
	peers, ok := rawPeers.([]any)
	if !ok {
		return nil, errors.New("peers must be an array")
	}
	out := make([]any, len(peers))
	for i, raw := range peers {
		peer, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("each peer must be an object")
		}
		copy, err := cloneMap(peer)
		if err != nil {
			return nil, err
		}
		normalizePeerAliases(copy)
		out[i] = copy
	}
	normalized["peers"] = out
	return normalized, nil
}

// TrimForRole extracts role-whitelisted fields from submitted content.
func TrimForRole(role string, content map[string]any) (patch map[string]any, ignored []string, err error) {
	if err := rejectSecrets(content); err != nil {
		return nil, nil, err
	}
	switch role {
	case "server":
		return trimServer(content)
	case "client":
		return trimClient(content)
	default:
		return nil, nil, fmt.Errorf("unsupported role %q", role)
	}
}

// EditorDraft projects live node config into the editable surface for a role.
func EditorDraft(role string, live map[string]any) (map[string]any, error) {
	if live == nil {
		return map[string]any{}, nil
	}
	switch role {
	case "server":
		draft := map[string]any{}
		if value, ok := live["allow_targets"]; ok {
			draft["allow_targets"] = value
		}
		if value, ok := live["deny_targets"]; ok {
			draft["deny_targets"] = value
		}
		if connectionConfig, ok := live["connection_config"].(map[string]any); ok {
			if desired, ok := connectionConfig["desired"].(map[string]any); ok {
				conn := map[string]any{}
				if compression, ok := desired["compression"].(map[string]any); ok {
					if level, ok := compression["level"]; ok {
						conn["compression"] = map[string]any{"level": level}
					}
				}
				if len(conn) > 0 {
					draft["connection"] = conn
				}
			}
		}
		return draft, nil
	case "client":
		normalized, err := NormalizeClientPeers(live)
		if err != nil {
			return nil, err
		}
		draft := map[string]any{}
		if peers, ok := normalized["peers"].([]any); ok {
			projected := make([]any, 0, len(peers))
			for _, raw := range peers {
				peer, ok := raw.(map[string]any)
				if !ok {
					return nil, errors.New("each peer must be an object")
				}
				ignored := []string{}
				trimmed, err := trimPeer(peer, &ignored)
				if err != nil {
					return nil, err
				}
				projected = append(projected, trimmed)
			}
			draft["peers"] = projected
		}
		return draft, nil
	default:
		return map[string]any{}, nil
	}
}

func trimServer(content map[string]any) (map[string]any, []string, error) {
	patch := map[string]any{}
	var ignored []string
	for key, value := range content {
		switch key {
		case "allow_targets", "deny_targets":
			if err := stringArray(key, value); err != nil {
				return nil, nil, err
			}
			patch[key] = value
		case "connection":
			connection, ok := value.(map[string]any)
			if !ok {
				return nil, nil, errors.New("connection must be an object")
			}
			connPatch := map[string]any{}
			for connectionKey, connectionValue := range connection {
				switch connectionKey {
				case "compression":
					compression, ok := connectionValue.(map[string]any)
					if !ok {
						return nil, nil, errors.New("connection.compression must be an object")
					}
					levelPatch := map[string]any{}
					for compressionKey, compressionValue := range compression {
						if compressionKey != "level" {
							ignored = append(ignored, "connection.compression."+compressionKey)
							continue
						}
						if err := validateCompressionLevel(compressionValue); err != nil {
							return nil, nil, err
						}
						levelPatch[compressionKey] = compressionValue
					}
					if len(levelPatch) > 0 {
						connPatch["compression"] = levelPatch
					}
				case "min_send_rate_kbps", "max_send_rate_kbps":
					ignored = append(ignored, "connection."+connectionKey)
				default:
					ignored = append(ignored, "connection."+connectionKey)
				}
			}
			if len(connPatch) > 0 {
				patch["connection"] = connPatch
			}
		default:
			ignored = append(ignored, key)
		}
	}
	return patch, ignored, nil
}

func trimClient(content map[string]any) (map[string]any, []string, error) {
	var ignored []string
	peersInput, hasPeers := content["peers"]
	for key := range content {
		if key != "peers" {
			ignored = append(ignored, key)
		}
	}
	if !hasPeers {
		return map[string]any{}, ignored, nil
	}
	peers, ok := peersInput.([]any)
	if !ok {
		return nil, nil, errors.New("peers must be an array")
	}
	if len(peers) == 0 {
		return map[string]any{}, ignored, nil
	}
	normalized := map[string]any{"peers": peers}
	normalized, err := NormalizeClientPeers(normalized)
	if err != nil {
		return nil, nil, err
	}
	outPeers, ok := normalized["peers"].([]any)
	if !ok {
		return nil, nil, errors.New("peers must be an array")
	}
	trimmedPeers := make([]any, 0, len(outPeers))
	for _, raw := range outPeers {
		peer, ok := raw.(map[string]any)
		if !ok {
			return nil, nil, errors.New("each peer must be an object")
		}
		trimmed, err := trimPeer(peer, &ignored)
		if err != nil {
			return nil, nil, err
		}
		trimmedPeers = append(trimmedPeers, trimmed)
	}
	return map[string]any{"peers": trimmedPeers}, ignored, nil
}

func trimPeer(peer map[string]any, ignored *[]string) (map[string]any, error) {
	out := map[string]any{}
	for key, value := range peer {
		if !allowedPeerFields[key] {
			*ignored = append(*ignored, key)
			continue
		}
		switch key {
		case "connection":
			connection, ok := value.(map[string]any)
			if !ok {
				return nil, errors.New("connection must be an object")
			}
			connPatch, err := trimPeerConnection(connection, ignored)
			if err != nil {
				return nil, err
			}
			if len(connPatch) > 0 {
				out[key] = connPatch
			}
		case "port_forwards":
			forwards, ok := value.([]any)
			if !ok {
				return nil, errors.New("port_forwards must be an array")
			}
			trimmed, err := trimPortForwards(forwards, ignored)
			if err != nil {
				return nil, err
			}
			out[key] = trimmed
		default:
			out[key] = value
		}
	}
	return out, nil
}

func trimPeerConnection(connection map[string]any, ignored *[]string) (map[string]any, error) {
	out := map[string]any{}
	for connectionKey, connectionValue := range connection {
		switch connectionKey {
		case "encryption":
			out[connectionKey] = connectionValue
		case "compression":
			compression, ok := connectionValue.(map[string]any)
			if !ok {
				return nil, errors.New("connection.compression must be an object")
			}
			compPatch := map[string]any{}
			for compressionKey, compressionValue := range compression {
				if compressionKey != "mode" && compressionKey != "level" {
					*ignored = append(*ignored, compressionKey)
					continue
				}
				compPatch[compressionKey] = compressionValue
			}
			if len(compPatch) > 0 {
				out["compression"] = compPatch
			}
		case "min_send_rate_kbps", "max_send_rate_kbps":
			*ignored = append(*ignored, connectionKey)
		default:
			*ignored = append(*ignored, connectionKey)
		}
	}
	return out, nil
}

func trimPortForwards(forwards []any, ignored *[]string) ([]any, error) {
	out := make([]any, len(forwards))
	for i, raw := range forwards {
		forward, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("each port_forward must be an object")
		}
		trimmed := map[string]any{}
		for forwardKey, forwardValue := range forward {
			if forwardKey != "listen" && forwardKey != "target" {
				*ignored = append(*ignored, forwardKey)
				continue
			}
			trimmed[forwardKey] = forwardValue
		}
		out[i] = trimmed
	}
	return out, nil
}

func normalizePeerAliases(peer map[string]any) {
	if id, ok := peer["id"].(string); ok && id != "" {
		if _, exists := peer["peer_id"]; !exists {
			peer["peer_id"] = id
		}
		delete(peer, "id")
	}
	if protoPeer, ok := peer["proto_peer"].(string); ok && protoPeer != "" {
		if _, exists := peer["quic_peer"]; !exists {
			peer["quic_peer"] = protoPeer
		}
		delete(peer, "proto_peer")
	}
	if protoConnections, ok := peer["proto_connections"]; ok {
		if _, exists := peer["quic_connections"]; !exists {
			peer["quic_connections"] = protoConnections
		}
		delete(peer, "proto_connections")
	}
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
				switch connectionKey {
				case "encryption":
				case "compression":
					compression, ok := connectionValue.(map[string]any)
					if !ok {
						return errors.New("connection.compression must be an object")
					}
					for compressionKey := range compression {
						if compressionKey != "mode" && compressionKey != "level" {
							return fmt.Errorf("connection.compression field %q is not allowed", compressionKey)
						}
					}
				default:
					return fmt.Errorf("connection field %q is not allowed", connectionKey)
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

func deepMergeMap(base, patch map[string]any) map[string]any {
	merged, err := cloneMap(base)
	if err != nil {
		return patch
	}
	for key, value := range patch {
		if patchMap, ok := value.(map[string]any); ok {
			if baseMap, ok := merged[key].(map[string]any); ok {
				merged[key] = deepMergeMap(baseMap, patchMap)
				continue
			}
		}
		merged[key] = value
	}
	return merged
}

func rejectSecrets(value any) error {
	return rejectSecretsIn(value, "")
}

func rejectSecretsIn(value any, parent string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
			if strings.Contains(normalized, "token") || strings.Contains(normalized, "password") ||
				strings.Contains(normalized, "secret") || normalized == "private_key" ||
				normalized == "tls_key" ||
				(normalized == "key" && strings.EqualFold(parent, "tls")) {
				return fmt.Errorf("%w: %s", errSecretField, key)
			}
			if err := rejectSecretsIn(child, key); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := rejectSecretsIn(child, parent); err != nil {
				return err
			}
		}
	}
	return nil
}

func redactValue(value any, parent string) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
			if strings.Contains(normalized, "token") || strings.Contains(normalized, "password") ||
				strings.Contains(normalized, "secret") || normalized == "private_key" ||
				(normalized == "key" && strings.EqualFold(parent, "tls")) {
				result[key] = "[REDACTED]"
			} else {
				result[key] = redactValue(child, key)
			}
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, child := range typed {
			result[i] = redactValue(child, parent)
		}
		return result
	default:
		return value
	}
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

func validateCompressionLevel(value any) error {
	var level float64
	switch typed := value.(type) {
	case int:
		level = float64(typed)
	case int8:
		level = float64(typed)
	case int16:
		level = float64(typed)
	case int32:
		level = float64(typed)
	case int64:
		level = float64(typed)
	case uint:
		level = float64(typed)
	case uint8:
		level = float64(typed)
	case uint16:
		level = float64(typed)
	case uint32:
		level = float64(typed)
	case uint64:
		level = float64(typed)
	case float32:
		level = float64(typed)
	case float64:
		level = typed
	default:
		return errors.New("connection.compression.level must be an integer between 1 and 22")
	}
	if math.Trunc(level) != level || level < 1 || level > 22 {
		return errors.New("connection.compression.level must be an integer between 1 and 22")
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
