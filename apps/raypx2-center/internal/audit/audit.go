package audit

import (
	"time"

	"github.com/pocketbase/pocketbase/core"
)

const (
	ActionAgentEnroll  = "agent.enroll"
	ActionProxyRequest = "proxy.request"
)

// RecordAgentEnroll stores a secret-free enrollment outcome.
func RecordAgentEnroll(app core.App, nodeID, ip string, success bool) error {
	collection, err := app.FindCollectionByNameOrId("audit_logs")
	if err != nil {
		return err
	}

	record := core.NewRecord(collection)
	record.Set("action", ActionAgentEnroll)
	if nodeID != "" {
		record.Set("node", nodeID)
	}
	record.Set("ip", ip)
	record.Set("request_summary", map[string]any{"success": success})
	return app.Save(record)
}

// RecordProxyRequest stores proxy metadata without request or response bodies.
func RecordProxyRequest(
	app core.App,
	actorID string,
	nodeID string,
	ip string,
	method string,
	path string,
	status int,
	latency time.Duration,
) error {
	collection, err := app.FindCollectionByNameOrId("audit_logs")
	if err != nil {
		return err
	}

	record := core.NewRecord(collection)
	record.Set("action", ActionProxyRequest)
	if actorID != "" {
		record.Set("actor", actorID)
	}
	if nodeID != "" {
		record.Set("node", nodeID)
	}
	record.Set("ip", ip)
	record.Set("request_summary", map[string]any{
		"method":     method,
		"path":       path,
		"status":     status,
		"latency_ms": latency.Milliseconds(),
	})
	return app.Save(record)
}
