package audit

import (
	"github.com/pocketbase/pocketbase/core"
)

const ActionAgentEnroll = "agent.enroll"

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
