package quickstartcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// InstallRequest mirrors the server's InstallIntegrationRequest.
type InstallRequest struct {
	RepoRef    string         `json:"repo_ref"`
	ProviderID string         `json:"provider_id,omitempty"`
	Inputs     map[string]any `json:"inputs,omitempty"`
	DryRun     bool           `json:"dry_run,omitempty"`
}

// InstallResponse mirrors the server's InstallIntegrationResponse.
type InstallResponse struct {
	ProviderID       string         `json:"provider_id"`
	RunID            string         `json:"run_id,omitempty"`
	RunURL           string         `json:"run_url,omitempty"`
	CompiledWorkflow map[string]any `json:"compiled_workflow,omitempty"`
	Hints            []string       `json:"hints,omitempty"`
}

// Client is a thin HTTP client around the install endpoint. The server URL
// + auth token come from the CLI flags / env (see cmd/yggdrasil/install.go).
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// NewClient returns a Client with sensible defaults (60s timeout).
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Install POSTs the request body to /api/v1/integrations/install. A non-2xx
// response is decoded into a structured error so the CLI can show the
// server's "input X is required" message verbatim.
func (c *Client) Install(ctx context.Context, req InstallRequest) (InstallResponse, error) {
	if c.BaseURL == "" {
		return InstallResponse{}, fmt.Errorf("server URL is required (set --server or YGGDRASIL_URL)")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return InstallResponse{}, fmt.Errorf("encode request: %w", err)
	}

	url := c.BaseURL + "/api/v1/integrations/install"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return InstallResponse{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if c.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return InstallResponse{}, fmt.Errorf("call %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return InstallResponse{}, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errPayload struct {
			Error string `json:"error"`
		}
		if jsonErr := json.Unmarshal(respBody, &errPayload); jsonErr == nil && errPayload.Error != "" {
			return InstallResponse{}, fmt.Errorf("server returned HTTP %d: %s", resp.StatusCode, errPayload.Error)
		}
		return InstallResponse{}, fmt.Errorf("server returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var out InstallResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return InstallResponse{}, fmt.Errorf("decode response: %w (body=%s)", err, string(respBody))
	}
	return out, nil
}
