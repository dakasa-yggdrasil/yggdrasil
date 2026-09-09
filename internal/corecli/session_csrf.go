package corecli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Core issues opaque human-session tokens with the ys_ prefix. This only
// selects the CSRF protocol; authentication and authorization remain in Core.
// JWTs and machine credentials do not use the human-session endpoint.
func needsSessionCSRF(method, token string) bool {
	if !strings.HasPrefix(token, "ys_") {
		return false
	}
	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// readSessionCSRF obtains the token from the authenticated API, never browser
// cookies. It is request-local: no config writes or cross-context token cache.
// Do not reuse Do's error-body decoding here: a session envelope can contain
// credentials and collaborator data, including when a response is malformed.
func readSessionCSRF(ctx context.Context, client *http.Client, baseURL, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/auth/session", nil)
	if err != nil {
		return "", fmt.Errorf("cannot build session CSRF request; no mutation was sent")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("session CSRF request stopped; no mutation was sent: %w", ctx.Err())
		}
		return "", fmt.Errorf("session CSRF request failed; no mutation was sent")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		detail := "cannot obtain session CSRF token; no mutation was sent"
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			detail = "session expired or unavailable; log in again; no mutation was sent"
		case resp.StatusCode >= 300 && resp.StatusCode < 400:
			detail = "session CSRF requests do not follow redirects; configure the canonical server URL; no mutation was sent"
		}
		return "", &APIError{Status: resp.StatusCode, Detail: detail}
	}
	const maxSessionResponseBytes = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSessionResponseBytes+1))
	if err != nil || len(body) > maxSessionResponseBytes {
		return "", fmt.Errorf("cannot read session CSRF response; no mutation was sent")
	}
	var session struct {
		Authenticated bool   `json:"authenticated"`
		CSRFToken     string `json:"csrf_token"`
	}
	if json.Unmarshal(body, &session) != nil {
		return "", fmt.Errorf("invalid session CSRF response; no mutation was sent")
	}
	if !session.Authenticated {
		return "", fmt.Errorf("session is not authenticated; log in again; no mutation was sent")
	}
	if strings.TrimSpace(session.CSRFToken) == "" || strings.ContainsAny(session.CSRFToken, "\r\n") {
		return "", fmt.Errorf("authenticated session did not provide a valid CSRF token; no mutation was sent")
	}
	return session.CSRFToken, nil
}
