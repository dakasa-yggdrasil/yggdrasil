package quickstartcli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The install endpoint is a state-changing POST. When the CLI authenticates
// with a session token (from `yggdrasil login`), the core enforces CSRF, so
// the client must fetch the per-session csrf_token from /api/v1/auth/session
// and echo it in the X-CSRF-Token header.

func TestInstall_FetchesAndSendsCSRFToken(t *testing.T) {
	var sentCSRF string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"authenticated": true, "csrf_token": "csrf-abc"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/integrations/install":
			sentCSRF = r.Header.Get("X-CSRF-Token")
			if sentCSRF == "" {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"csrf required"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"provider_id": "x"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "session-tok")
	if _, err := c.Install(context.Background(), InstallRequest{RepoRef: "o/r", ProviderID: "x", DryRun: true}); err != nil {
		t.Fatalf("install: %v", err)
	}
	if sentCSRF != "csrf-abc" {
		t.Errorf("X-CSRF-Token = %q, want csrf-abc (fetched from /auth/session)", sentCSRF)
	}
}

func TestInstall_NoTokenSkipsCSRFFetch(t *testing.T) {
	sessionCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/session" {
			sessionCalled = true
		}
		if r.URL.Path == "/api/v1/integrations/install" {
			_ = json.NewEncoder(w).Encode(map[string]any{"provider_id": "x"})
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "") // no token → nothing to CSRF-protect
	if _, err := c.Install(context.Background(), InstallRequest{RepoRef: "o/r"}); err != nil {
		t.Fatalf("install: %v", err)
	}
	if sessionCalled {
		t.Error("should not hit /auth/session when no token is set")
	}
}
