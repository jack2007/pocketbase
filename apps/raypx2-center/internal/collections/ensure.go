package collections

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
)

var configRevisionSources = []string{
	"pull", "template_apply", "manual_edit", "proxy_write", "peer_delete",
}

// EnsureCollections creates the private collections used by raypx2-center.
func EnsureCollections(app core.App) error {
	superusers, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return err
	}

	nodes, err := ensure(app, "nodes", func(collection *core.Collection) {
		collection.Fields.Add(
			&core.TextField{Name: "node_key", Required: true},
			&core.TextField{Name: "name"},
			&core.SelectField{Name: "role", Values: []string{"client", "server", "unknown"}},
			&core.TextField{Name: "enroll_secret_hash", Hidden: true, Required: true},
			&core.TextField{Name: "grant_mac_key", Hidden: true},
			&core.SelectField{Name: "enroll_status", Values: []string{"active", "revoked"}},
			&core.JSONField{Name: "labels"},
			&core.TextField{Name: "hostname"},
			&core.TextField{Name: "agent_version"},
			&core.DateField{Name: "last_seen_at"},
			&core.BoolField{Name: "online"},
			relation("created_by", superusers, false, false),
		)
		collection.AddIndex("idx_nodes_node_key", true, "node_key", "")
	})
	if err != nil {
		return err
	}
	if err := ensureHiddenTextField(app, nodes, "grant_mac_key"); err != nil {
		return err
	}

	if _, err := ensure(app, "agent_sessions", func(collection *core.Collection) {
		collection.Fields.Add(
			relation("node", nodes, true, true),
			&core.TextField{Name: "token_hash", Hidden: true, Required: true},
			&core.DateField{Name: "expires_at", Required: true},
			&core.DateField{Name: "revoked_at"},
			&core.JSONField{Name: "client_info"},
		)
		collection.AddIndex("idx_agent_sessions_token_hash", true, "token_hash", "")
	}); err != nil {
		return err
	}

	if _, err := ensure(app, "node_status", func(collection *core.Collection) {
		collection.Fields.Add(
			relation("node", nodes, true, true),
			&core.TextField{Name: "health_status"},
			&core.NumberField{Name: "uptime_seconds", OnlyInt: true},
			&core.TextField{Name: "last_error"},
			&core.JSONField{Name: "summary"},
			&core.TextField{Name: "config_hash"},
			&core.DateField{Name: "fetched_at"},
		)
		collection.AddIndex("idx_node_status_node", true, "node", "")
	}); err != nil {
		return err
	}

	configRevisions, err := ensure(app, "config_revisions", func(collection *core.Collection) {
		collection.Fields.Add(
			relation("node", nodes, true, false),
			&core.SelectField{Name: "kind", Values: []string{"actual", "desired"}},
			&core.SelectField{Name: "source", Values: configRevisionSources},
			&core.TextField{Name: "content_hash"},
			&core.JSONField{Name: "content"},
			&core.TextField{Name: "diff_summary"},
			relation("actor", superusers, false, false),
		)
	})
	if err != nil {
		return err
	}
	if err := ensureSelectValues(app, configRevisions, "source", configRevisionSources); err != nil {
		return err
	}

	configTemplates, err := ensure(app, "config_templates", func(collection *core.Collection) {
		collection.Fields.Add(
			&core.TextField{Name: "name", Required: true},
			&core.SelectField{Name: "target_role", Values: []string{"client", "server", "unknown"}},
			&core.JSONField{Name: "body", Required: true},
			&core.NumberField{Name: "version", OnlyInt: true},
			&core.TextField{Name: "notes"},
		)
	})
	if err != nil {
		return err
	}

	applyJobs, err := ensure(app, "apply_jobs", func(collection *core.Collection) {
		collection.Fields.Add(
			relation("template", configTemplates, true, false),
			&core.NumberField{Name: "template_version", OnlyInt: true},
			&core.TextField{Name: "status"},
			relation("created_by", superusers, false, false),
		)
	})
	if err != nil {
		return err
	}

	if _, err := ensure(app, "apply_job_targets", func(collection *core.Collection) {
		collection.Fields.Add(
			relation("job", applyJobs, true, true),
			relation("node", nodes, true, false),
			&core.TextField{Name: "status"},
			&core.TextField{Name: "error"},
			relation("result_revision", configRevisions, false, false),
		)
	}); err != nil {
		return err
	}

	_, err = ensure(app, "audit_logs", func(collection *core.Collection) {
		collection.Fields.Add(
			relation("actor", superusers, false, false),
			&core.TextField{Name: "action", Required: true},
			relation("node", nodes, false, false),
			&core.JSONField{Name: "request_summary"},
			&core.TextField{Name: "ip"},
		)
	})
	return err
}

func ensure(app core.App, name string, configure func(*core.Collection)) (*core.Collection, error) {
	if collection, err := app.FindCollectionByNameOrId(name); err == nil {
		if addAutodates(collection) {
			if err := app.Save(collection); err != nil {
				return nil, err
			}
		}
		return collection, nil
	}

	collection := core.NewBaseCollection(name)
	configure(collection)
	addAutodates(collection)
	if err := app.Save(collection); err != nil {
		return nil, err
	}
	return collection, nil
}

func addAutodates(collection *core.Collection) bool {
	changed := false
	if collection.Fields.GetByName("created") == nil {
		collection.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		changed = true
	}
	if collection.Fields.GetByName("updated") == nil {
		collection.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		changed = true
	}
	return changed
}

func ensureHiddenTextField(app core.App, collection *core.Collection, name string) error {
	if collection.Fields.GetByName(name) != nil {
		return nil
	}
	collection.Fields.Add(&core.TextField{Name: name, Hidden: true})
	return app.Save(collection)
}

func ensureSelectValues(app core.App, collection *core.Collection, fieldName string, want []string) error {
	field, ok := collection.Fields.GetByName(fieldName).(*core.SelectField)
	if !ok || field == nil {
		return nil
	}
	changed := false
	for _, value := range want {
		if !slices.Contains(field.Values, value) {
			field.Values = append(field.Values, value)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return app.Save(collection)
}

func relation(name string, collection *core.Collection, required, cascade bool) *core.RelationField {
	return &core.RelationField{
		Name:          name,
		CollectionId:  collection.Id,
		MaxSelect:     1,
		Required:      required,
		CascadeDelete: cascade,
	}
}
