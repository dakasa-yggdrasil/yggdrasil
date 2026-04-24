package corecli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is the shared HTTP client used by login/apply/get/describe/
// logs/status to talk to a yggdrasil-core. It centralizes bearer auth,
// Accept/Content-Type headers, and error-body decoding so subcommands
// do not re-implement those details.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// NewClient constructs a Client aimed at the active context.
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// FromContext is the common pattern used by subcommands: resolve the
// active context and build a client for it, with optional explicit
// overrides (flags take precedence over persisted config).
func FromContext(server, token string) (*Client, error) {
	cfg, _, err := Load()
	if err != nil {
		return nil, err
	}
	active, _ := cfg.Active()
	if active == nil {
		active = &Context{}
	}
	if server == "" {
		server = active.Server
	}
	if token == "" {
		token = active.Token
	}
	if server == "" {
		return nil, fmt.Errorf("no server configured: run `yggdrasil init` or `yggdrasil login --server=<url>`")
	}
	return NewClient(server, token), nil
}

// APIError carries the server's typed error message so subcommands can
// show "input X is required" verbatim instead of "HTTP 400".
type APIError struct {
	Status int
	Body   string
	Detail string
}

func (e *APIError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("server returned HTTP %d: %s", e.Status, e.Detail)
	}
	return fmt.Sprintf("server returned HTTP %d: %s", e.Status, e.Body)
}

// Do executes the request and unmarshals the response body into out
// (when non-nil). Non-2xx responses become *APIError.
func (c *Client) Do(ctx context.Context, method, path string, body any, out any) error {
	if c.BaseURL == "" {
		return fmt.Errorf("server URL is required")
	}
	target := c.BaseURL + path
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("call %s %s: %w", method, target, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		apiErr := &APIError{Status: resp.StatusCode, Body: string(respBody)}
		var payload struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &payload) == nil && payload.Error != "" {
			apiErr.Detail = payload.Error
		}
		return apiErr
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w (body=%s)", err, string(respBody))
		}
	}
	return nil
}

// Login exchanges credentials for a session token. Returns the token
// plus the collaborator slug (convenient to persist in Context).
func (c *Client) Login(ctx context.Context, identifier, password string) (string, string, error) {
	req := map[string]any{"identifier": identifier, "password": password}
	var resp struct {
		Token        string `json:"token"`
		Collaborator struct {
			Slug string `json:"slug"`
		} `json:"collaborator"`
	}
	if err := c.Do(ctx, http.MethodPost, "/api/v1/auth/login", req, &resp); err != nil {
		return "", "", err
	}
	if resp.Token == "" {
		return "", "", fmt.Errorf("server did not return a session token")
	}
	return resp.Token, resp.Collaborator.Slug, nil
}

// Healthz returns nil when the server is ready to accept traffic.
// Used by `yggdrasil init` to poll during docker-compose boot and by
// `yggdrasil status` for quick checks.
func (c *Client) Healthz(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("healthz returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Readyz mirrors Healthz but hits /readyz (which typically asserts DB
// connectivity). Used after docker-compose up to confirm the service
// actually works end-to-end, not just that the process started.
func (c *Client) Readyz(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/readyz", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("readyz returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// CreateManifest POSTs a manifest document through the generic
// /api/v1/manifests?kind=<kind> endpoint. Returns the server-assigned
// record. The kind travels as a query parameter so one URL covers
// every manifest shape — no per-kind URL table in the CLI.
func (c *Client) CreateManifest(ctx context.Context, kind string, payload any) (map[string]any, error) {
	var resp struct {
		Manifest map[string]any `json:"manifest"`
	}
	if err := c.Do(ctx, http.MethodPost, "/api/v1/manifests?kind="+url.QueryEscape(kind), payload, &resp); err != nil {
		return nil, err
	}
	return resp.Manifest, nil
}

// ListManifests hits /api/v1/manifests?kind=<kind>&namespace=&name=
// and returns the raw list payload for the caller to format.
func (c *Client) ListManifests(ctx context.Context, kind string, namespace, name string, activeOnly bool) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("kind", kind)
	if namespace != "" {
		q.Set("namespace", namespace)
	}
	if name != "" {
		q.Set("name", name)
	}
	if activeOnly {
		q.Set("active_only", "true")
	}
	var resp struct {
		Manifests []map[string]any `json:"manifests"`
	}
	if err := c.Do(ctx, http.MethodGet, "/api/v1/manifests?"+q.Encode(), nil, &resp); err != nil {
		return nil, err
	}
	return resp.Manifests, nil
}
