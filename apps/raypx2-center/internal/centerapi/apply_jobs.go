package centerapi

import (
	"net/http"

	applyrunner "github.com/pocketbase/pocketbase/apps/raypx2-center/internal/apply"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/audit"
	"github.com/pocketbase/pocketbase/core"
)

type createApplyJobRequest struct {
	Template string   `json:"template"`
	Nodes    []string `json:"nodes"`
}

func (api *API) HandleListApplyJobs(e *core.RequestEvent) error {
	jobs, err := e.App.FindRecordsByFilter("apply_jobs", "", "", 1000, 0)
	if err != nil {
		return e.InternalServerError("Failed to list apply jobs.", err)
	}
	items := make([]map[string]any, 0, len(jobs))
	for _, job := range jobs {
		item, err := applyJobView(e.App, job)
		if err != nil {
			return e.InternalServerError("Failed to load apply job.", err)
		}
		items = append(items, item)
	}
	return e.JSON(http.StatusOK, map[string]any{"items": items})
}

func (api *API) HandleGetApplyJob(e *core.RequestEvent) error {
	job, err := e.App.FindRecordById("apply_jobs", e.Request.PathValue("job_id"))
	if err != nil {
		return e.JSON(http.StatusNotFound, errorResponse("apply_job_not_found"))
	}
	item, err := applyJobView(e.App, job)
	if err != nil {
		return e.InternalServerError("Failed to load apply job.", err)
	}
	return e.JSON(http.StatusOK, item)
}

func (api *API) HandleCreateApplyJob(e *core.RequestEvent) error {
	var request createApplyJobRequest
	if err := e.BindBody(&request); err != nil || request.Template == "" || len(request.Nodes) == 0 {
		return e.JSON(http.StatusBadRequest, errorResponse("invalid_apply_job"))
	}
	template, err := e.App.FindRecordById("config_templates", request.Template)
	if err != nil {
		return e.JSON(http.StatusBadRequest, errorResponse("template_not_found"))
	}
	nodes := make([]*core.Record, 0, len(request.Nodes))
	seen := make(map[string]bool, len(request.Nodes))
	for _, nodeID := range request.Nodes {
		if seen[nodeID] {
			continue
		}
		node, err := e.App.FindRecordById("nodes", nodeID)
		if err != nil {
			return e.JSON(http.StatusBadRequest, errorResponse("node_not_found"))
		}
		if role := template.GetString("target_role"); role != node.GetString("role") {
			return e.JSON(http.StatusBadRequest, errorResponse("template_role_mismatch"))
		}
		seen[nodeID] = true
		nodes = append(nodes, node)
	}

	var job *core.Record
	err = e.App.RunInTransaction(func(txApp core.App) error {
		jobCollection, err := txApp.FindCollectionByNameOrId("apply_jobs")
		if err != nil {
			return err
		}
		job = core.NewRecord(jobCollection)
		job.Set("template", template.Id)
		job.Set("template_version", template.GetInt("version"))
		job.Set("status", "queued")
		if e.Auth != nil {
			job.Set("created_by", e.Auth.Id)
		}
		if err := txApp.Save(job); err != nil {
			return err
		}
		targetCollection, err := txApp.FindCollectionByNameOrId("apply_job_targets")
		if err != nil {
			return err
		}
		for _, node := range nodes {
			target := core.NewRecord(targetCollection)
			target.Set("job", job.Id)
			target.Set("node", node.Id)
			target.Set("status", "queued")
			if err := txApp.Save(target); err != nil {
				return err
			}
		}
		return audit.RecordManagement(
			txApp, actorID(e), audit.ActionApplyJobCreate, "", e.RemoteIP(),
			map[string]any{
				"job_id":           job.Id,
				"template_id":      template.Id,
				"template_version": template.GetInt("version"),
				"target_count":     len(nodes),
			},
		)
	})
	if err != nil {
		return e.InternalServerError("Failed to create apply job.", err)
	}
	start := api.startApply
	if start == nil {
		start = func(jobID string) error { return applyrunner.StartApplyJob(e.App, jobID) }
	}
	if err := start(job.Id); err != nil {
		job.Set("status", "failed")
		_ = e.App.Save(job)
		return e.InternalServerError("Failed to start apply job.", err)
	}
	item, err := applyJobView(e.App, job)
	if err != nil {
		return e.InternalServerError("Failed to load apply job.", err)
	}
	return e.JSON(http.StatusCreated, item)
}

func applyJobView(app core.App, job *core.Record) (map[string]any, error) {
	targets, err := app.FindRecordsByFilter(
		"apply_job_targets", "job = {:job}", "", 1000, 0, map[string]any{"job": job.Id},
	)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":               job.Id,
		"template":         job.GetString("template"),
		"template_version": job.GetInt("template_version"),
		"status":           job.GetString("status"),
		"created_by":       job.GetString("created_by"),
		"targets":          targets,
	}, nil
}
