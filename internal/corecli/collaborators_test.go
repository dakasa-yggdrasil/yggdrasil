package corecli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Helper to build a fake yggdrasil-core that serves the canned envelope on
// (method, path) and asserts the request body when one is expected.
type recordedRequest struct {
	method string
	path   string
	body   map[string]any
}

type fakeServer struct {
	server *httptest.Server
	last   recordedRequest
}

func newFakeServer(t *testing.T, handler http.HandlerFunc) *fakeServer {
	t.Helper()
	fs := &fakeServer{}
	fs.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.last.method = r.Method
		fs.last.path = r.URL.RequestURI()
		if r.Body != nil {
			b, _ := io.ReadAll(r.Body)
			if len(b) > 0 {
				_ = json.Unmarshal(b, &fs.last.body)
			}
		}
		handler(w, r)
	}))
	t.Cleanup(fs.server.Close)
	return fs
}

func (fs *fakeServer) client() *Client {
	return NewClient(fs.server.URL, "test-token")
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func sampleCollaborator() Collaborator {
	return Collaborator{
		ID:           "00000000-0000-0000-0000-000000000001",
		Slug:         "alice",
		Status:       "active",
		DisplayName:  "Alice",
		PrimaryEmail: "alice@example.com",
	}
}

// --- CreateCollaborator ---------------------------------------------------

func TestCreateCollaborator_HappyPath(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusCreated, collaboratorEnvelope{Collaborator: sampleCollaborator()})
	})
	got, err := fs.client().CreateCollaborator(context.Background(), CreateCollaboratorRequest{
		Slug:        "alice",
		DisplayName: "Alice",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.Slug != "alice" || got.Status != "active" {
		t.Errorf("unexpected collaborator: %+v", got)
	}
	if fs.last.method != http.MethodPost || fs.last.path != "/api/v1/collaborators" {
		t.Errorf("wrong request: %s %s", fs.last.method, fs.last.path)
	}
	if fs.last.body["slug"] != "alice" {
		t.Errorf("slug not sent in body: %+v", fs.last.body)
	}
}

func TestCreateCollaborator_ServerError(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusBadRequest, map[string]any{"detail": "slug required"})
	})
	_, err := fs.client().CreateCollaborator(context.Background(), CreateCollaboratorRequest{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest {
		t.Fatalf("expected APIError 400, got %v", err)
	}
}

// --- GetCollaborator ------------------------------------------------------

func TestGetCollaborator_FoundBySlug(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorsEnvelope{Collaborators: []Collaborator{sampleCollaborator()}})
	})
	got, err := fs.client().GetCollaborator(context.Background(), "alice")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Slug != "alice" {
		t.Errorf("got %q, want alice", got.Slug)
	}
}

func TestGetCollaborator_NotFound(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorsEnvelope{Collaborators: []Collaborator{sampleCollaborator()}})
	})
	_, err := fs.client().GetCollaborator(context.Background(), "ghost")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
		t.Fatalf("expected 404 APIError, got %v", err)
	}
}

// --- ListCollaborators ----------------------------------------------------

func TestListCollaborators_NoFilter(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorsEnvelope{Collaborators: []Collaborator{
			sampleCollaborator(),
			{ID: "id2", Slug: "bob", Status: "offboarded"},
		}})
	})
	cs, err := fs.client().ListCollaborators(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cs) != 2 {
		t.Errorf("got %d, want 2", len(cs))
	}
	if !strings.HasPrefix(fs.last.path, "/api/v1/collaborators") || strings.Contains(fs.last.path, "status=") {
		t.Errorf("path should not have status filter: %s", fs.last.path)
	}
}

func TestListCollaborators_WithStatusFilter(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorsEnvelope{Collaborators: nil})
	})
	if _, err := fs.client().ListCollaborators(context.Background(), "active"); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(fs.last.path, "status=active") {
		t.Errorf("status=active not propagated: %s", fs.last.path)
	}
}

// --- UpdateCollaborator ---------------------------------------------------

func TestUpdateCollaborator_PatchesAndReturns(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		updated := sampleCollaborator()
		updated.DisplayName = "Alice II"
		writeJSON(t, w, http.StatusOK, collaboratorEnvelope{Collaborator: updated})
	})
	got, err := fs.client().UpdateCollaborator(context.Background(), "alice", map[string]any{
		"display_name": "Alice II",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.DisplayName != "Alice II" {
		t.Errorf("wrong display: %s", got.DisplayName)
	}
	if fs.last.method != http.MethodPatch || fs.last.path != "/api/v1/collaborators/alice" {
		t.Errorf("wrong req: %s %s", fs.last.method, fs.last.path)
	}
	if fs.last.body["id"] != "alice" || fs.last.body["display_name"] != "Alice II" {
		t.Errorf("body missing fields: %+v", fs.last.body)
	}
}

// --- OffboardCollaborator -------------------------------------------------

func TestOffboardCollaborator_AllFields(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorEnvelope{Collaborator: sampleCollaborator()})
	})
	if _, err := fs.client().OffboardCollaborator(context.Background(), "alice", "voluntary", "2026-12-31", 30); err != nil {
		t.Fatalf("offboard: %v", err)
	}
	if fs.last.path != "/api/v1/collaborators/alice/offboard" {
		t.Errorf("path: %s", fs.last.path)
	}
	if fs.last.body["reason"] != "voluntary" || fs.last.body["end_date"] != "2026-12-31" || fs.last.body["voluntary_notice_days"].(float64) != 30 {
		t.Errorf("body: %+v", fs.last.body)
	}
}

func TestOffboardCollaborator_OmitsZeroValues(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorEnvelope{Collaborator: sampleCollaborator()})
	})
	if _, err := fs.client().OffboardCollaborator(context.Background(), "alice", "involuntary", "", 0); err != nil {
		t.Fatalf("offboard: %v", err)
	}
	if _, ok := fs.last.body["end_date"]; ok {
		t.Errorf("end_date should be omitted: %+v", fs.last.body)
	}
	if _, ok := fs.last.body["voluntary_notice_days"]; ok {
		t.Errorf("notice days should be omitted: %+v", fs.last.body)
	}
}

// --- Suspend / Unsuspend (status verbs) -----------------------------------

func TestSuspendCollaborator_PassesReason(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorEnvelope{Collaborator: sampleCollaborator()})
	})
	if _, err := fs.client().SuspendCollaborator(context.Background(), "alice", "investigation"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if fs.last.path != "/api/v1/collaborators/alice/suspend" || fs.last.body["reason"] != "investigation" {
		t.Errorf("suspend req: %s %+v", fs.last.path, fs.last.body)
	}
}

func TestUnsuspendCollaborator_AllowsEmptyReason(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorEnvelope{Collaborator: sampleCollaborator()})
	})
	if _, err := fs.client().UnsuspendCollaborator(context.Background(), "alice", ""); err != nil {
		t.Fatalf("unsuspend: %v", err)
	}
	if fs.last.path != "/api/v1/collaborators/alice/unsuspend" {
		t.Errorf("path: %s", fs.last.path)
	}
	if _, ok := fs.last.body["reason"]; ok {
		t.Errorf("reason should be omitted when empty: %+v", fs.last.body)
	}
}

// --- ReOnboardCollaborator ------------------------------------------------

func TestReOnboardCollaborator_OptionalFields(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorEnvelope{Collaborator: sampleCollaborator()})
	})
	if _, err := fs.client().ReOnboardCollaborator(context.Background(), "alice", "2027-01-01", "engineer"); err != nil {
		t.Fatalf("re-onboard: %v", err)
	}
	if fs.last.path != "/api/v1/collaborators/alice/re-onboard" {
		t.Errorf("path: %s", fs.last.path)
	}
	if fs.last.body["new_start_date"] != "2027-01-01" || fs.last.body["role"] != "engineer" {
		t.Errorf("body: %+v", fs.last.body)
	}
}

// --- ChangeRole / ChangeManager ------------------------------------------

func TestChangeRole_SendsNewRole(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorEnvelope{Collaborator: sampleCollaborator()})
	})
	if _, err := fs.client().ChangeRole(context.Background(), "alice", "lead"); err != nil {
		t.Fatalf("role-change: %v", err)
	}
	if fs.last.path != "/api/v1/collaborators/alice/role-change" || fs.last.body["new_role"] != "lead" {
		t.Errorf("role-change: %s %+v", fs.last.path, fs.last.body)
	}
}

func TestChangeManager_SendsNewManager(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorEnvelope{Collaborator: sampleCollaborator()})
	})
	if _, err := fs.client().ChangeManager(context.Background(), "alice", "manager-1"); err != nil {
		t.Fatalf("manager-change: %v", err)
	}
	if fs.last.path != "/api/v1/collaborators/alice/manager-change" || fs.last.body["new_manager_id"] != "manager-1" {
		t.Errorf("manager-change: %s %+v", fs.last.path, fs.last.body)
	}
}

// --- AddTeam / RemoveTeam ------------------------------------------------

func TestAddTeam_OptionalRoleInTeam(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorEnvelope{Collaborator: sampleCollaborator()})
	})
	if _, err := fs.client().AddTeam(context.Background(), "alice", "team-1", "lead"); err != nil {
		t.Fatalf("team-add: %v", err)
	}
	if fs.last.path != "/api/v1/collaborators/alice/team-add" || fs.last.body["team_id"] != "team-1" || fs.last.body["role_in_team"] != "lead" {
		t.Errorf("team-add: %s %+v", fs.last.path, fs.last.body)
	}
}

func TestAddTeam_OmitsEmptyRoleInTeam(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorEnvelope{Collaborator: sampleCollaborator()})
	})
	if _, err := fs.client().AddTeam(context.Background(), "alice", "team-1", ""); err != nil {
		t.Fatalf("team-add: %v", err)
	}
	if _, ok := fs.last.body["role_in_team"]; ok {
		t.Errorf("role_in_team should be omitted: %+v", fs.last.body)
	}
}

func TestRemoveTeam_SendsTeamID(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorEnvelope{Collaborator: sampleCollaborator()})
	})
	if _, err := fs.client().RemoveTeam(context.Background(), "alice", "team-1"); err != nil {
		t.Fatalf("team-remove: %v", err)
	}
	if fs.last.path != "/api/v1/collaborators/alice/team-remove" || fs.last.body["team_id"] != "team-1" {
		t.Errorf("team-remove: %s %+v", fs.last.path, fs.last.body)
	}
}

// --- SetAttribute ---------------------------------------------------------

func TestSetAttribute_AllFields(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorEnvelope{Collaborator: sampleCollaborator()})
	})
	if _, err := fs.client().SetAttribute(context.Background(), "alice", "level", "senior", "string"); err != nil {
		t.Fatalf("attribute-set: %v", err)
	}
	if fs.last.path != "/api/v1/collaborators/alice/attribute-set" {
		t.Errorf("path: %s", fs.last.path)
	}
	if fs.last.body["key"] != "level" || fs.last.body["value"] != "senior" || fs.last.body["value_type"] != "string" {
		t.Errorf("body: %+v", fs.last.body)
	}
}

func TestSetAttribute_OmitsEmptyValueType(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorEnvelope{Collaborator: sampleCollaborator()})
	})
	if _, err := fs.client().SetAttribute(context.Background(), "alice", "on_call", true, ""); err != nil {
		t.Fatalf("attribute-set: %v", err)
	}
	if _, ok := fs.last.body["value_type"]; ok {
		t.Errorf("value_type should be omitted: %+v", fs.last.body)
	}
}

// --- StartAbsence / EndAbsence -------------------------------------------

func TestStartAbsence_WithDuration(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorEnvelope{Collaborator: sampleCollaborator()})
	})
	if _, err := fs.client().StartAbsence(context.Background(), "alice", "vacation", "2026-07-01", "2026-07-30", 30); err != nil {
		t.Fatalf("absence-start: %v", err)
	}
	if fs.last.path != "/api/v1/collaborators/alice/absence/start" {
		t.Errorf("path: %s", fs.last.path)
	}
	if fs.last.body["type"] != "vacation" || fs.last.body["from"] != "2026-07-01" || fs.last.body["to"] != "2026-07-30" || fs.last.body["duration_days"].(float64) != 30 {
		t.Errorf("body: %+v", fs.last.body)
	}
}

func TestEndAbsence_OptionalActualEnd(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorEnvelope{Collaborator: sampleCollaborator()})
	})
	if _, err := fs.client().EndAbsence(context.Background(), "alice", "evt-1", "2026-07-25"); err != nil {
		t.Fatalf("absence-end: %v", err)
	}
	if fs.last.path != "/api/v1/collaborators/alice/absence/end" {
		t.Errorf("path: %s", fs.last.path)
	}
	if fs.last.body["absence_event_id"] != "evt-1" || fs.last.body["actual_end"] != "2026-07-25" {
		t.Errorf("body: %+v", fs.last.body)
	}
}

// --- ListLifecycleEvents --------------------------------------------------

func TestListLifecycleEvents_AppliesFilters(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, lifecycleEventsEnvelope{Events: []LifecycleEvent{
			{ID: "evt-1", CollaboratorID: "alice", EventType: "hired"},
		}})
	})
	events, err := fs.client().ListLifecycleEvents(context.Background(), "alice", "hired", 50)
	if err != nil {
		t.Fatalf("lifecycle-events: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "hired" {
		t.Errorf("events: %+v", events)
	}
	if !strings.Contains(fs.last.path, "/api/v1/collaborators/alice/lifecycle-events") {
		t.Errorf("base path: %s", fs.last.path)
	}
	if !strings.Contains(fs.last.path, "event_type=hired") || !strings.Contains(fs.last.path, "limit=50") {
		t.Errorf("filters not propagated: %s", fs.last.path)
	}
}

func TestListLifecycleEvents_NoFilters(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, lifecycleEventsEnvelope{Events: nil})
	})
	if _, err := fs.client().ListLifecycleEvents(context.Background(), "alice", "", 0); err != nil {
		t.Fatalf("lifecycle-events: %v", err)
	}
	if strings.Contains(fs.last.path, "?") {
		t.Errorf("query string should be empty: %s", fs.last.path)
	}
}

// --- GetProviderState ----------------------------------------------------

func TestGetProviderState_Returns(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, providerStateEnvelope{ProviderState: []CollaboratorProviderState{
			{CollaboratorID: "alice", Provider: "google-workspace", ExternalID: "gws-1"},
		}})
	})
	got, err := fs.client().GetProviderState(context.Background(), "alice")
	if err != nil {
		t.Fatalf("provider-state: %v", err)
	}
	if len(got) != 1 || got[0].Provider != "google-workspace" {
		t.Errorf("state: %+v", got)
	}
	if fs.last.method != http.MethodGet || fs.last.path != "/api/v1/collaborators/alice/provider-state" {
		t.Errorf("req: %s %s", fs.last.method, fs.last.path)
	}
}

// --- Path-escape sanity ---------------------------------------------------

func TestCollaboratorIDIsURLEscaped(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, collaboratorEnvelope{Collaborator: sampleCollaborator()})
	})
	if _, err := fs.client().SuspendCollaborator(context.Background(), "alice/space", "x"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if !strings.Contains(fs.last.path, "alice%2Fspace") {
		t.Errorf("id not escaped: %s", fs.last.path)
	}
}
