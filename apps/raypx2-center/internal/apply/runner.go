// Package apply executes multi-node configuration template jobs.
package apply

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/agenthub"
	"github.com/pocketbase/pocketbase/apps/raypx2-center/internal/configmerge"
	"github.com/pocketbase/pocketbase/core"
)

const targetTimeout = 20 * time.Second

type ProxyRequester interface {
	RequestProxy(context.Context, string, agenthub.ProxyRequest) (agenthub.ProxyResponse, error)
}

type Runner struct {
	app   core.App
	proxy ProxyRequester
}

var defaultProxy struct {
	sync.RWMutex
	requester ProxyRequester
}

func NewRunner(app core.App, proxy ProxyRequester) *Runner {
	return &Runner{app: app, proxy: proxy}
}

// SetProxyRequester configures the process-wide proxy used by StartApplyJob.
func SetProxyRequester(proxy ProxyRequester) {
	defaultProxy.Lock()
	defaultProxy.requester = proxy
	defaultProxy.Unlock()
}

// StartApplyJob validates the job and starts it asynchronously.
func StartApplyJob(app core.App, jobID string) error {
	if _, err := app.FindRecordById("apply_jobs", jobID); err != nil {
		return err
	}
	defaultProxy.RLock()
	proxy := defaultProxy.requester
	defaultProxy.RUnlock()
	if proxy == nil {
		return errors.New("apply proxy is not configured")
	}
	runner := NewRunner(app, proxy)
	go func() {
		if err := runner.RunJob(context.Background(), jobID); err != nil {
			app.Logger().Error("apply job failed", "job", jobID, "error", err)
		}
	}()
	return nil
}

// RunJob executes targets sequentially and persists target/job terminal states.
func (r *Runner) RunJob(ctx context.Context, jobID string) error {
	job, err := r.app.FindRecordById("apply_jobs", jobID)
	if err != nil {
		return err
	}
	template, err := r.app.FindRecordById("config_templates", job.GetString("template"))
	if err != nil {
		return r.failJob(job, fmt.Errorf("template not found: %w", err))
	}
	if job.GetInt("template_version") != template.GetInt("version") {
		return r.failJob(job, errors.New("template_version_changed"))
	}
	templateBody, err := objectValue(template.Get("body"))
	if err != nil {
		return r.failJob(job, err)
	}
	targets, err := r.app.FindRecordsByFilter(
		"apply_job_targets", "job = {:job}", "", 1000, 0, map[string]any{"job": jobID},
	)
	if err != nil {
		return err
	}
	job.Set("status", "running")
	if err := r.app.Save(job); err != nil {
		return err
	}

	completed := 0
	for _, target := range targets {
		target.Set("status", "running")
		target.Set("error", "")
		if err := r.app.Save(target); err != nil {
			return err
		}
		if err := r.applyTarget(ctx, job, target, template, templateBody); err != nil {
			target.Set("status", "failed")
			target.Set("error", errorCode(err))
		} else {
			target.Set("status", "completed")
			completed++
		}
		if err := r.app.Save(target); err != nil {
			return err
		}
	}

	switch {
	case len(targets) == 0 || completed == 0:
		job.Set("status", "failed")
	case completed == len(targets):
		job.Set("status", "completed")
	default:
		job.Set("status", "partial")
	}
	return r.app.Save(job)
}

func (r *Runner) applyTarget(
	ctx context.Context,
	job, target, template *core.Record,
	templateBody map[string]any,
) error {
	node, err := r.app.FindRecordById("nodes", target.GetString("node"))
	if err != nil {
		return fmt.Errorf("node_not_found: %w", err)
	}
	role := node.GetString("role")
	var path, writeMethod string
	switch role {
	case "server":
		path, writeMethod = "/api/v1/server/config", http.MethodPatch
	case "client":
		path, writeMethod = "/api/v1/config", http.MethodPut
	default:
		return errors.New("unsupported_node_role")
	}
	if targetRole := template.GetString("target_role"); targetRole != "" && targetRole != "unknown" && targetRole != role {
		return errors.New("template_role_mismatch")
	}

	targetCtx, cancel := context.WithTimeout(ctx, targetTimeout)
	defer cancel()
	actual, err := r.proxyJSON(targetCtx, node.GetString("node_key"), http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if _, err := r.saveRevision(node, job, "actual", "pull", actual); err != nil {
		return err
	}

	var desired, writeBody map[string]any
	switch role {
	case "server":
		desired, err = configmerge.MergeServerACL(actual, templateBody)
		writeBody = templateBody
	case "client":
		desired, err = configmerge.MergeClientPeers(actual, templateBody)
		writeBody = desired
	}
	if err != nil {
		return err
	}
	if _, err := r.proxyJSON(targetCtx, node.GetString("node_key"), writeMethod, path, writeBody); err != nil {
		return err
	}
	revision, err := r.saveRevision(node, job, "desired", "template_apply", desired)
	if err != nil {
		return err
	}
	target.Set("result_revision", revision.Id)
	return nil
}

func (r *Runner) proxyJSON(
	ctx context.Context,
	nodeKey, method, path string,
	body map[string]any,
) (map[string]any, error) {
	var bodyB64 string
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyB64 = base64.StdEncoding.EncodeToString(encoded)
	}
	response, err := r.proxy.RequestProxy(ctx, nodeKey, agenthub.ProxyRequest{
		Method: method, Path: path, BodyB64: bodyB64,
		Headers:   map[string]string{"Content-Type": "application/json"},
		TimeoutMS: int(targetTimeout / time.Millisecond),
	})
	if err != nil {
		return nil, err
	}
	if response.Error != "" {
		return nil, errors.New(response.Error)
	}
	if response.Status < 200 || response.Status >= 300 {
		return nil, fmt.Errorf("admin_status_%d", response.Status)
	}
	decoded, err := base64.StdEncoding.DecodeString(response.BodyB64)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if len(decoded) == 0 {
		return map[string]any{}, nil
	}
	if err := json.Unmarshal(decoded, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Runner) saveRevision(
	node, job *core.Record,
	kind, source string,
	content map[string]any,
) (*core.Record, error) {
	safe := redact(content, "")
	encoded, err := json.Marshal(safe)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(encoded)
	collection, err := r.app.FindCollectionByNameOrId("config_revisions")
	if err != nil {
		return nil, err
	}
	revision := core.NewRecord(collection)
	revision.Set("node", node.Id)
	revision.Set("kind", kind)
	revision.Set("source", source)
	revision.Set("content_hash", hex.EncodeToString(sum[:]))
	revision.Set("content", safe)
	revision.Set("diff_summary", fmt.Sprintf("apply job %s", job.Id))
	if actor := job.GetString("created_by"); actor != "" {
		revision.Set("actor", actor)
	}
	if err := r.app.Save(revision); err != nil {
		return nil, err
	}
	return revision, nil
}

func (r *Runner) failJob(job *core.Record, cause error) error {
	job.Set("status", "failed")
	if err := r.app.Save(job); err != nil {
		return err
	}
	return cause
}

func objectValue(value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		return nil, errors.New("template body must be an object")
	}
	if object == nil {
		return nil, errors.New("template body must be an object")
	}
	return object, nil
}

func redact(value any, parent string) any {
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
				result[key] = redact(child, key)
			}
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, child := range typed {
			result[i] = redact(child, parent)
		}
		return result
	default:
		return value
	}
}

func errorCode(err error) string {
	switch {
	case errors.Is(err, agenthub.ErrNodeOffline):
		return "node_offline"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "tunnel_timeout"
	default:
		message := err.Error()
		if len(message) > 500 {
			message = message[:500]
		}
		return message
	}
}
