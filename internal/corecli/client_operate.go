package corecli

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// This file holds the HTTP client methods that back the "operate" half of
// the CLI — running workflows (async), managing runs (ops list/get/
// retry/abort/replay), and deleting manifests. The "declare" half
// (CreateManifest / ListManifests / RunWorkflow) lives in client.go.

// OpsRun is one row of GET /api/v1/ops/workflows (mirrors
// model.OpsWorkflowRun in yggdrasil-core).
type OpsRun struct {
	RunID         string `json:"run_id"`
	WorkflowName  string `json:"workflow_name"`
	Integration   string `json:"integration"`
	Status        string `json:"status"`
	TriggerSource string `json:"trigger_source"`
	DurationMS    *int64 `json:"duration_ms"`
	Error         string `json:"error,omitempty"`
}

// OpsRunsPage is the GET /api/v1/ops/workflows response envelope.
type OpsRunsPage struct {
	Runs       []OpsRun `json:"runs"`
	NextCursor string   `json:"next_cursor,omitempty"`
}

// OpsListOptions are the supported filters on the ops list endpoint.
type OpsListOptions struct {
	Status      []string
	Integration string
	Search      string
	Cursor      string
	Limit       int
}

// ListRuns returns a page of workflow runs from /api/v1/ops/workflows.
func (c *Client) ListRuns(ctx context.Context, opts OpsListOptions) (OpsRunsPage, error) {
	q := url.Values{}
	for _, s := range opts.Status {
		if s != "" {
			q.Add("status", s)
		}
	}
	if opts.Integration != "" {
		q.Set("integration", opts.Integration)
	}
	if opts.Search != "" {
		q.Set("search", opts.Search)
	}
	if opts.Cursor != "" {
		q.Set("cursor", opts.Cursor)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	var page OpsRunsPage
	if err := c.Do(ctx, http.MethodGet, "/api/v1/ops/workflows?"+q.Encode(), nil, &page); err != nil {
		return OpsRunsPage{}, err
	}
	return page, nil
}

// GetRun fetches one run's detail from /api/v1/ops/workflows/{runId}.
// The shape is intentionally returned raw (steps/inputs/outputs are
// free-form) so the CLI can render whatever the core reports.
func (c *Client) GetRun(ctx context.Context, runID string) (map[string]any, error) {
	var out map[string]any
	if err := c.Do(ctx, http.MethodGet, "/api/v1/ops/workflows/"+url.PathEscape(runID), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// opsVerbs is the closed set of run lifecycle actions the core exposes.
var opsVerbs = map[string]bool{"retry": true, "abort": true, "replay": true}

// RunOp POSTs a lifecycle action against a run. verb must be one of
// retry|abort|replay; reason is optional and only consumed by retry.
func (c *Client) RunOp(ctx context.Context, runID, verb, reason string) (map[string]any, error) {
	if !opsVerbs[verb] {
		return nil, fmt.Errorf("unknown run operation %q (want retry|abort|replay)", verb)
	}
	var body any
	if reason != "" {
		body = map[string]any{"reason": reason}
	}
	var out map[string]any
	if err := c.Do(ctx, http.MethodPost, "/api/v1/ops/workflows/"+url.PathEscape(runID)+"/"+verb, body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteManifest removes a manifest by id. soft=true flips active=FALSE on
// every version sharing the (kind, namespace, name); the default hard-
// deletes those rows. Mirrors DELETE /api/v1/manifests/{id}[?soft=true].
func (c *Client) DeleteManifest(ctx context.Context, id string, soft bool) (map[string]any, error) {
	path := "/api/v1/manifests/" + url.PathEscape(id)
	if soft {
		path += "?soft=true"
	}
	var out map[string]any
	if err := c.Do(ctx, http.MethodDelete, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RunWorkflowMode dispatches a workflow run, opting into async execution
// when async=true (?async=true → 202 + {run_id, status}). The sync path
// returns the full Steps; async returns RunID for later polling.
func (c *Client) RunWorkflowMode(ctx context.Context, namespace, name string, inputs map[string]any, async bool) (WorkflowRunResult, error) {
	path := "/api/v1/workflow-runs"
	if async {
		path += "?async=true"
	}
	payload := map[string]any{
		"workflow": map[string]any{"namespace": namespace, "name": name},
	}
	if inputs != nil {
		payload["inputs"] = inputs
	}
	var result WorkflowRunResult
	if err := c.Do(ctx, http.MethodPost, path, payload, &result); err != nil {
		return WorkflowRunResult{}, err
	}
	return result, nil
}
