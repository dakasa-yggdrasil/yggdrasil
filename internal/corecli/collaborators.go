package corecli

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// Collaborator and Team mirror the JSON shape returned by yggdrasil-core
// REST. Kept in a thin DTO form to avoid importing the core repo.
type Collaborator struct {
	ID                   string                 `json:"id"`
	Slug                 string                 `json:"slug"`
	Status               string                 `json:"status"`
	DisplayName          string                 `json:"display_name"`
	PrimaryEmail         string                 `json:"primary_email"`
	ManagerID            string                 `json:"manager_id,omitempty"`
	PrimaryTeamID        string                 `json:"primary_team_id,omitempty"`
	PersonalData         map[string]any         `json:"personal_data,omitempty"`
	EmploymentData       map[string]any         `json:"employment_data,omitempty"`
	ThirdPartyIdentities map[string]any         `json:"third_party_identities,omitempty"`
	Traits               map[string]any         `json:"traits,omitempty"`
	Metadata             map[string]any         `json:"metadata,omitempty"`
	CreatedAt            string                 `json:"created_at,omitempty"`
	UpdatedAt            string                 `json:"updated_at,omitempty"`
}

type LifecycleEvent struct {
	ID             string         `json:"id"`
	CollaboratorID string         `json:"collaborator_id"`
	EventType      string         `json:"event_type"`
	Payload        map[string]any `json:"payload"`
	ActorType      string         `json:"actor_type"`
	ActorID        string         `json:"actor_id"`
	OccurredAt     string         `json:"occurred_at"`
	EffectiveAt    string         `json:"effective_at"`
}

type CollaboratorProviderState struct {
	CollaboratorID       string         `json:"collaborator_id"`
	Provider             string         `json:"provider"`
	ExternalID           string         `json:"external_id,omitempty"`
	DesiredState         map[string]any `json:"desired_state,omitempty"`
	ObservedState        map[string]any `json:"observed_state,omitempty"`
	LastReconciledAt     string         `json:"last_reconciled_at,omitempty"`
	LastDriftDetectedAt  string         `json:"last_drift_detected_at,omitempty"`
	PendingAction        string         `json:"pending_action,omitempty"`
	ErrorCount           int            `json:"error_count"`
	LastError            string         `json:"last_error,omitempty"`
}

type CreateCollaboratorRequest struct {
	Slug           string         `json:"slug"`
	Status         string         `json:"status,omitempty"`
	DisplayName    string         `json:"display_name"`
	PrimaryEmail   string         `json:"primary_email,omitempty"`
	ManagerID      string         `json:"manager_id,omitempty"`
	PrimaryTeamID  string         `json:"primary_team_id,omitempty"`
	EmploymentData map[string]any `json:"employment_data,omitempty"`
	Traits         map[string]any `json:"traits,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type collaboratorEnvelope struct {
	Collaborator Collaborator `json:"collaborator"`
}
type collaboratorsEnvelope struct {
	Collaborators []Collaborator `json:"collaborators"`
}
type lifecycleEventsEnvelope struct {
	Events []LifecycleEvent `json:"events"`
}
type providerStateEnvelope struct {
	ProviderState []CollaboratorProviderState `json:"provider_state"`
}

func (c *Client) CreateCollaborator(ctx context.Context, req CreateCollaboratorRequest) (Collaborator, error) {
	var env collaboratorEnvelope
	if err := c.Do(ctx, http.MethodPost, "/api/v1/collaborators", req, &env); err != nil {
		return Collaborator{}, err
	}
	return env.Collaborator, nil
}

func (c *Client) GetCollaborator(ctx context.Context, id string) (Collaborator, error) {
	// REST has GET /api/v1/collaborators (list); for "get one" we filter via list.
	// Until a dedicated GET endpoint lands we rely on caller passing slug or ID;
	// this mirrors how repository.GetCollaborator resolves both.
	collabs, err := c.ListCollaborators(ctx, "")
	if err != nil {
		return Collaborator{}, err
	}
	for _, col := range collabs {
		if col.ID == id || col.Slug == id || col.PrimaryEmail == id {
			return col, nil
		}
	}
	return Collaborator{}, &APIError{Status: http.StatusNotFound, Body: "collaborator not found: " + id, Detail: "collaborator not found: " + id}
}

func (c *Client) ListCollaborators(ctx context.Context, statusFilter string) ([]Collaborator, error) {
	q := url.Values{}
	if statusFilter != "" {
		q.Set("status", statusFilter)
	}
	path := "/api/v1/collaborators"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var env collaboratorsEnvelope
	if err := c.Do(ctx, http.MethodGet, path, nil, &env); err != nil {
		return nil, err
	}
	return env.Collaborators, nil
}

func (c *Client) UpdateCollaborator(ctx context.Context, id string, body map[string]any) (Collaborator, error) {
	body["id"] = id
	var env collaboratorEnvelope
	if err := c.Do(ctx, http.MethodPatch, "/api/v1/collaborators/"+url.PathEscape(id), body, &env); err != nil {
		return Collaborator{}, err
	}
	return env.Collaborator, nil
}

func (c *Client) OffboardCollaborator(ctx context.Context, id string, reason, endDate string, voluntaryNoticeDays int) (Collaborator, error) {
	body := map[string]any{"reason": reason}
	if endDate != "" {
		body["end_date"] = endDate
	}
	if voluntaryNoticeDays > 0 {
		body["voluntary_notice_days"] = voluntaryNoticeDays
	}
	var env collaboratorEnvelope
	err := c.Do(ctx, http.MethodPost, "/api/v1/collaborators/"+url.PathEscape(id)+"/offboard", body, &env)
	return env.Collaborator, err
}

func (c *Client) SuspendCollaborator(ctx context.Context, id, reason string) (Collaborator, error) {
	return c.statusVerb(ctx, id, "suspend", reason)
}
func (c *Client) UnsuspendCollaborator(ctx context.Context, id, reason string) (Collaborator, error) {
	return c.statusVerb(ctx, id, "unsuspend", reason)
}
func (c *Client) ReOnboardCollaborator(ctx context.Context, id, newStartDate, role string) (Collaborator, error) {
	body := map[string]any{}
	if newStartDate != "" {
		body["new_start_date"] = newStartDate
	}
	if role != "" {
		body["role"] = role
	}
	var env collaboratorEnvelope
	err := c.Do(ctx, http.MethodPost, "/api/v1/collaborators/"+url.PathEscape(id)+"/re-onboard", body, &env)
	return env.Collaborator, err
}

func (c *Client) statusVerb(ctx context.Context, id, verb, reason string) (Collaborator, error) {
	body := map[string]any{}
	if reason != "" {
		body["reason"] = reason
	}
	var env collaboratorEnvelope
	err := c.Do(ctx, http.MethodPost, "/api/v1/collaborators/"+url.PathEscape(id)+"/"+verb, body, &env)
	return env.Collaborator, err
}

func (c *Client) ChangeRole(ctx context.Context, id, newRole string) (Collaborator, error) {
	body := map[string]any{"new_role": newRole}
	var env collaboratorEnvelope
	err := c.Do(ctx, http.MethodPost, "/api/v1/collaborators/"+url.PathEscape(id)+"/role-change", body, &env)
	return env.Collaborator, err
}

func (c *Client) AddTeam(ctx context.Context, id, teamID, roleInTeam string) (Collaborator, error) {
	body := map[string]any{"team_id": teamID}
	if roleInTeam != "" {
		body["role_in_team"] = roleInTeam
	}
	var env collaboratorEnvelope
	err := c.Do(ctx, http.MethodPost, "/api/v1/collaborators/"+url.PathEscape(id)+"/team-add", body, &env)
	return env.Collaborator, err
}

func (c *Client) RemoveTeam(ctx context.Context, id, teamID string) (Collaborator, error) {
	body := map[string]any{"team_id": teamID}
	var env collaboratorEnvelope
	err := c.Do(ctx, http.MethodPost, "/api/v1/collaborators/"+url.PathEscape(id)+"/team-remove", body, &env)
	return env.Collaborator, err
}

func (c *Client) SetAttribute(ctx context.Context, id, key string, value any, valueType string) (Collaborator, error) {
	body := map[string]any{"key": key, "value": value}
	if valueType != "" {
		body["value_type"] = valueType
	}
	var env collaboratorEnvelope
	err := c.Do(ctx, http.MethodPost, "/api/v1/collaborators/"+url.PathEscape(id)+"/attribute-set", body, &env)
	return env.Collaborator, err
}

func (c *Client) ChangeManager(ctx context.Context, id, newManagerID string) (Collaborator, error) {
	body := map[string]any{"new_manager_id": newManagerID}
	var env collaboratorEnvelope
	err := c.Do(ctx, http.MethodPost, "/api/v1/collaborators/"+url.PathEscape(id)+"/manager-change", body, &env)
	return env.Collaborator, err
}

func (c *Client) StartAbsence(ctx context.Context, id, absenceType, from, to string, durationDays int) (Collaborator, error) {
	body := map[string]any{"type": absenceType}
	if from != "" {
		body["from"] = from
	}
	if to != "" {
		body["to"] = to
	}
	if durationDays > 0 {
		body["duration_days"] = durationDays
	}
	var env collaboratorEnvelope
	err := c.Do(ctx, http.MethodPost, "/api/v1/collaborators/"+url.PathEscape(id)+"/absence/start", body, &env)
	return env.Collaborator, err
}

func (c *Client) EndAbsence(ctx context.Context, id, absenceEventID, actualEnd string) (Collaborator, error) {
	body := map[string]any{}
	if absenceEventID != "" {
		body["absence_event_id"] = absenceEventID
	}
	if actualEnd != "" {
		body["actual_end"] = actualEnd
	}
	var env collaboratorEnvelope
	err := c.Do(ctx, http.MethodPost, "/api/v1/collaborators/"+url.PathEscape(id)+"/absence/end", body, &env)
	return env.Collaborator, err
}

func (c *Client) ListLifecycleEvents(ctx context.Context, id, eventType string, limit int) ([]LifecycleEvent, error) {
	q := url.Values{}
	if eventType != "" {
		q.Set("event_type", eventType)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := "/api/v1/collaborators/" + url.PathEscape(id) + "/lifecycle-events"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var env lifecycleEventsEnvelope
	if err := c.Do(ctx, http.MethodGet, path, nil, &env); err != nil {
		return nil, err
	}
	return env.Events, nil
}

func (c *Client) GetProviderState(ctx context.Context, id string) ([]CollaboratorProviderState, error) {
	var env providerStateEnvelope
	if err := c.Do(ctx, http.MethodGet, "/api/v1/collaborators/"+url.PathEscape(id)+"/provider-state", nil, &env); err != nil {
		return nil, err
	}
	return env.ProviderState, nil
}
