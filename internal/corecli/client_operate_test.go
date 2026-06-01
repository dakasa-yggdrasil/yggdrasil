package corecli

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// These cover the client methods that back the operate-half CLI commands
// (run / delete / ops). They use the shared newFakeServer + writeJSON
// helpers defined in collaborators_test.go.

// --- ListRuns -------------------------------------------------------------

func TestListRuns_HappyPath(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"runs": []map[string]any{
				{"run_id": "run-1", "workflow_name": "deploy", "status": "succeeded"},
				{"run_id": "run-2", "workflow_name": "deploy", "status": "failed"},
			},
			"next_cursor": "abc",
		})
	})
	page, err := fs.client().ListRuns(context.Background(), OpsListOptions{Limit: 50, Status: []string{"failed"}})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(page.Runs) != 2 || page.Runs[0].RunID != "run-1" || page.Runs[1].Status != "failed" {
		t.Errorf("unexpected runs: %+v", page.Runs)
	}
	if page.NextCursor != "abc" {
		t.Errorf("next_cursor not parsed: %q", page.NextCursor)
	}
	if fs.last.method != http.MethodGet || !strings.HasPrefix(fs.last.path, "/api/v1/ops/workflows?") {
		t.Errorf("wrong request: %s %s", fs.last.method, fs.last.path)
	}
	if !strings.Contains(fs.last.path, "limit=50") || !strings.Contains(fs.last.path, "status=failed") {
		t.Errorf("filters not in query: %s", fs.last.path)
	}
}

// --- GetRun ---------------------------------------------------------------

func TestGetRun_HappyPath(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"run_id": "run-1", "workflow_name": "deploy", "status": "succeeded",
			"steps": []any{map[string]any{"id": "s1", "status": "succeeded"}},
		})
	})
	got, err := fs.client().GetRun(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got["run_id"] != "run-1" || got["status"] != "succeeded" {
		t.Errorf("unexpected detail: %+v", got)
	}
	if fs.last.method != http.MethodGet || fs.last.path != "/api/v1/ops/workflows/run-1" {
		t.Errorf("wrong request: %s %s", fs.last.method, fs.last.path)
	}
}

// --- RunOp (retry/abort/replay) -------------------------------------------

func TestRunOp_Retry(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusAccepted, map[string]any{"new_run_id": "retry_xyz", "source": "run-1"})
	})
	got, err := fs.client().RunOp(context.Background(), "run-1", "retry", "because")
	if err != nil {
		t.Fatalf("RunOp: %v", err)
	}
	if got["new_run_id"] != "retry_xyz" {
		t.Errorf("new_run_id not parsed: %+v", got)
	}
	if fs.last.method != http.MethodPost || fs.last.path != "/api/v1/ops/workflows/run-1/retry" {
		t.Errorf("wrong request: %s %s", fs.last.method, fs.last.path)
	}
	if fs.last.body["reason"] != "because" {
		t.Errorf("reason not sent: %+v", fs.last.body)
	}
}

func TestRunOp_RejectsUnknownVerb(t *testing.T) {
	c := NewClient("http://example.invalid", "")
	if _, err := c.RunOp(context.Background(), "run-1", "frobnicate", ""); err == nil {
		t.Fatal("expected error for unknown verb")
	}
}

// --- DeleteManifest -------------------------------------------------------

func TestDeleteManifest_HardByDefault(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{"deleted": true, "mode": "hard"})
	})
	got, err := fs.client().DeleteManifest(context.Background(), "id-123", false)
	if err != nil {
		t.Fatalf("DeleteManifest: %v", err)
	}
	if got["deleted"] != true {
		t.Errorf("unexpected response: %+v", got)
	}
	if fs.last.method != http.MethodDelete || fs.last.path != "/api/v1/manifests/id-123" {
		t.Errorf("wrong request (hard should carry no soft query): %s %s", fs.last.method, fs.last.path)
	}
}

func TestDeleteManifest_SoftAddsQuery(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{"deleted": true, "mode": "soft"})
	})
	if _, err := fs.client().DeleteManifest(context.Background(), "id-123", true); err != nil {
		t.Fatalf("DeleteManifest soft: %v", err)
	}
	if fs.last.path != "/api/v1/manifests/id-123?soft=true" {
		t.Errorf("soft query missing: %s", fs.last.path)
	}
}

// --- RunWorkflowMode (async dispatch) -------------------------------------

func TestRunWorkflowMode_AsyncSetsQueryAndParsesRunID(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusAccepted, map[string]any{"run_id": "run-async-1", "status": "pending"})
	})
	res, err := fs.client().RunWorkflowMode(context.Background(), "global", "deploy", map[string]any{"k": "v"}, true)
	if err != nil {
		t.Fatalf("RunWorkflowMode: %v", err)
	}
	if res.RunID != "run-async-1" || res.Status != "pending" {
		t.Errorf("unexpected result: %+v", res)
	}
	if fs.last.path != "/api/v1/workflow-runs?async=true" {
		t.Errorf("async query missing: %s", fs.last.path)
	}
	if wf, _ := fs.last.body["workflow"].(map[string]any); wf["name"] != "deploy" {
		t.Errorf("workflow name not sent: %+v", fs.last.body)
	}
}
