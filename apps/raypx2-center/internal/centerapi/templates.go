package centerapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/audit"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/configmerge"
	"github.com/pocketbase/pocketbase/core"
)

type templateRequest struct {
	Name       string         `json:"name"`
	TargetRole string         `json:"target_role"`
	Body       map[string]any `json:"body"`
	Notes      string         `json:"notes"`
}

func (api *API) HandleListTemplates(e *core.RequestEvent) error {
	records, err := e.App.FindRecordsByFilter("config_templates", "", "", 1000, 0)
	if err != nil {
		return e.InternalServerError("Failed to list templates.", err)
	}
	return e.JSON(http.StatusOK, map[string]any{"items": records})
}

func (api *API) HandleCreateTemplate(e *core.RequestEvent) error {
	request, err := bindAndValidateTemplate(e)
	if err != nil {
		return e.JSON(http.StatusBadRequest, errorResponse("invalid_template"))
	}
	var record *core.Record
	err = e.App.RunInTransaction(func(txApp core.App) error {
		collection, err := txApp.FindCollectionByNameOrId("config_templates")
		if err != nil {
			return err
		}
		record = core.NewRecord(collection)
		setTemplate(record, request)
		record.Set("version", 1)
		if err := txApp.Save(record); err != nil {
			return err
		}
		return audit.RecordManagement(
			txApp, actorID(e), audit.ActionTemplateCreate, "", e.RemoteIP(),
			map[string]any{
				"template_id": record.Id,
				"name":        record.GetString("name"),
				"target_role": record.GetString("target_role"),
				"version":     record.GetInt("version"),
			},
		)
	})
	if err != nil {
		return e.JSON(http.StatusBadRequest, errorResponse("invalid_template"))
	}
	return e.JSON(http.StatusCreated, record)
}

func (api *API) HandleUpdateTemplate(e *core.RequestEvent) error {
	if _, err := e.App.FindRecordById("config_templates", e.Request.PathValue("template_id")); err != nil {
		return e.JSON(http.StatusNotFound, errorResponse("template_not_found"))
	}
	request, err := bindAndValidateTemplate(e)
	if err != nil {
		return e.JSON(http.StatusBadRequest, errorResponse("invalid_template"))
	}
	var record *core.Record
	err = e.App.RunInTransaction(func(txApp core.App) error {
		record, err = txApp.FindRecordById("config_templates", e.Request.PathValue("template_id"))
		if err != nil {
			return err
		}
		setTemplate(record, request)
		record.Set("version", record.GetInt("version")+1)
		if err := txApp.Save(record); err != nil {
			return err
		}
		return audit.RecordManagement(
			txApp, actorID(e), audit.ActionTemplateUpdate, "", e.RemoteIP(),
			map[string]any{
				"template_id": record.Id,
				"name":        record.GetString("name"),
				"target_role": record.GetString("target_role"),
				"version":     record.GetInt("version"),
			},
		)
	})
	if err != nil {
		return e.JSON(http.StatusBadRequest, errorResponse("invalid_template"))
	}
	return e.JSON(http.StatusOK, record)
}

func (api *API) HandleDeleteTemplate(e *core.RequestEvent) error {
	record, err := e.App.FindRecordById("config_templates", e.Request.PathValue("template_id"))
	if err != nil {
		return e.JSON(http.StatusNotFound, errorResponse("template_not_found"))
	}
	if err := e.App.Delete(record); err != nil {
		return e.JSON(http.StatusConflict, errorResponse("template_in_use"))
	}
	return e.NoContent(http.StatusNoContent)
}

func bindAndValidateTemplate(e *core.RequestEvent) (templateRequest, error) {
	var request templateRequest
	if err := e.BindBody(&request); err != nil {
		return request, err
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || request.Body == nil {
		return request, errInvalidTemplate
	}
	switch request.TargetRole {
	case "server":
		_, err := configmerge.MergeServerACL(map[string]any{}, request.Body)
		return request, err
	case "client":
		_, err := configmerge.MergeClientPeers(map[string]any{"peers": []any{}}, request.Body)
		return request, err
	default:
		return request, errInvalidTemplate
	}
}

func setTemplate(record *core.Record, request templateRequest) {
	record.Set("name", request.Name)
	record.Set("target_role", request.TargetRole)
	record.Set("body", request.Body)
	record.Set("notes", request.Notes)
}

var errInvalidTemplate = errors.New("invalid template")
