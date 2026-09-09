package corecli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSessionMutationsFetchAndSendCSRF(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, "post"} {
		t.Run(method, func(t *testing.T) {
			var reads, mutations atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer ys_session_fixture" {
					t.Error("request did not use the explicit session bearer")
				}
				if r.Header.Get("Cookie") != "" {
					t.Error("request must not use ambient cookies")
				}
				if r.Header.Get("Accept") != "application/json" {
					t.Error("request did not accept JSON")
				}
				if r.URL.Path == "/api/v1/auth/session" {
					reads.Add(1)
					if r.Method != http.MethodGet || r.Header.Get("X-CSRF-Token") != "" {
						t.Error("session lookup must be a GET without a borrowed CSRF token")
					}
					http.SetCookie(w, &http.Cookie{Name: "yggdrasil_csrf_token", Value: "cookie-not-the-contract", Path: "/"})
					writeJSON(t, w, http.StatusOK, map[string]any{"authenticated": true, "csrf_token": "csrf_body_fixture"})
					return
				}
				mutations.Add(1)
				if reads.Load() != 1 || r.Method != method || r.URL.RequestURI() != "/api/v1/manifests?kind=workflow" {
					t.Error("mutation changed or was sent before session lookup")
				}
				if r.Header.Get("X-CSRF-Token") != "csrf_body_fixture" || r.Header.Get("Content-Type") != "application/json" {
					t.Error("mutation is missing CSRF or JSON content type")
				}
				var body map[string]string
				if json.NewDecoder(r.Body).Decode(&body) != nil || body["name"] != "fixture" {
					t.Error("mutation body changed")
				}
				writeJSON(t, w, http.StatusCreated, map[string]string{"id": "created"})
			}))
			defer server.Close()
			client := NewClient(server.URL, "ys_session_fixture")
			jar, err := cookiejar.New(nil)
			if err != nil {
				t.Fatal(err)
			}
			serverURL, _ := url.Parse(server.URL)
			jar.SetCookies(serverURL, []*http.Cookie{{Name: "yggdrasil_session", Value: "wrong-ambient-session"}})
			client.HTTPClient.Jar = jar
			var out map[string]string
			if err := client.Do(context.Background(), method, "/api/v1/manifests?kind=workflow", map[string]string{"name": "fixture"}, &out); err != nil {
				t.Fatal(err)
			}
			if mutations.Load() != 1 || out["id"] != "created" {
				t.Error("expected exactly one mutation and the original response")
			}
			if client.HTTPClient.Jar != jar || len(jar.Cookies(serverURL)) != 1 {
				t.Error("session request changed the caller's client or cookie jar")
			}
		})
	}
}

func TestSessionCSRFDoesNotChangeReadsOrOtherCredentials(t *testing.T) {
	for _, tc := range []struct{ method, token string }{
		{http.MethodGet, "ys_session_fixture"},
		{http.MethodHead, "ys_session_fixture"},
		{http.MethodOptions, "ys_session_fixture"},
		{http.MethodPost, "eyJ.jwt.fixture"},
		{http.MethodPost, "machine-token-fixture"},
		{http.MethodPost, ""},
	} {
		t.Run(tc.method+"/"+tc.token, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/api/v1/fixture" || r.Method != tc.method || r.Header.Get("X-CSRF-Token") != "" {
					t.Error("read or non-session request unexpectedly used the CSRF protocol")
				}
				wantAuth := ""
				if tc.token != "" {
					wantAuth = "Bearer " + tc.token
				}
				if r.Header.Get("Authorization") != wantAuth {
					t.Error("original bearer authentication changed")
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			if err := NewClient(server.URL, tc.token).Do(context.Background(), tc.method, "/api/v1/fixture", nil, nil); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 {
				t.Errorf("expected one request, got %d", calls.Load())
			}
		})
	}
}

func TestSessionCSRFFailsClosedWithoutExposingSessionBody(t *testing.T) {
	const secret = "private-session-response-fixture"
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"expired", 401, `{"error":"` + secret + `"}`},
		{"forbidden", 403, secret},
		{"unavailable", 503, secret},
		{"redirect", 302, secret},
		{"empty-success", 204, ""},
		{"invalid-json", 200, `{"csrf_token":"` + secret + `",broken}`},
		{"trailing-json", 200, `{"authenticated":true,"csrf_token":"` + secret + `"} {}`},
		{"unauthenticated", 200, `{"authenticated":false,"csrf_token":"` + secret + `"}`},
		{"missing-authenticated", 200, `{"csrf_token":"` + secret + `"}`},
		{"missing-csrf", 200, `{"authenticated":true,"collaborator":"` + secret + `"}`},
		{"blank-csrf", 200, `{"authenticated":true,"csrf_token":" \t "}`},
		{"invalid-header", 200, `{"authenticated":true,"csrf_token":"` + secret + `\r\n"}`},
		{"wrong-type", 200, `{"authenticated":true,"csrf_token":{"private":"` + secret + `"}}`},
		{"oversized", 200, strings.Repeat(secret, (1<<20)/len(secret)+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mutations atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/auth/session" {
					mutations.Add(1)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			err := NewClient(server.URL, "ys_session_fixture").Do(context.Background(), http.MethodPost, "/api/v1/manifests", nil, nil)
			if err == nil || !strings.Contains(err.Error(), "no mutation was sent") {
				t.Fatalf("expected actionable closed failure, got %v", err)
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "ys_session_fixture") {
				t.Error("error exposed session response or bearer")
			}
			var apiErr *APIError
			if errors.As(err, &apiErr) && apiErr.Body != "" {
				t.Error("typed error retained a private session body")
			}
			if mutations.Load() != 0 {
				t.Error("mutation was sent without authenticated CSRF")
			}
		})
	}
}

func TestSessionCSRFRefreshesAcrossTokensAndServers(t *testing.T) {
	var reads, mutations atomic.Int32
	newServer := func(id string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			csrf := id + "/" + r.Header.Get("Authorization")
			if r.URL.Path == "/api/v1/auth/session" {
				reads.Add(1)
				writeJSON(t, w, 200, map[string]any{"authenticated": true, "csrf_token": csrf})
				return
			}
			mutations.Add(1)
			if r.Header.Get("X-CSRF-Token") != csrf {
				t.Error("CSRF token was reused across a server or identity change")
			}
			w.WriteHeader(204)
		}))
	}
	first, second := newServer("first"), newServer("second")
	defer first.Close()
	defer second.Close()
	client := NewClient(first.URL, "ys_first_fixture")
	for _, tc := range []struct{ server, token string }{
		{first.URL, "ys_first_fixture"},
		{first.URL, "ys_second_fixture"},
		{second.URL, "ys_second_fixture"},
	} {
		client.BaseURL, client.Token = tc.server, tc.token
		if err := client.Do(context.Background(), http.MethodPost, "/api/v1/manifests", nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if reads.Load() != 3 || mutations.Load() != 3 {
		t.Error("every mutation must obtain its own server-bound session CSRF token")
	}
}

func TestSessionCSRFDoesNotFollowRedirectsOrReplayMutations(t *testing.T) {
	for _, phase := range []string{"session", "mutation"} {
		for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
			t.Run(phase+"/"+http.StatusText(status), func(t *testing.T) {
				var redirected, mutations atomic.Int32
				destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					redirected.Add(1)
					w.WriteHeader(204)
				}))
				defer destination.Close()
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/v1/auth/session" && phase == "mutation" {
						writeJSON(t, w, 200, map[string]any{"authenticated": true, "csrf_token": "csrf_fixture"})
						return
					}
					if r.Method == http.MethodPost {
						mutations.Add(1)
					}
					http.Redirect(w, r, destination.URL+"/?private=redirect-secret-fixture", status)
				}))
				defer server.Close()
				client := NewClient(server.URL, "ys_session_fixture")
				originalClient := client.HTTPClient
				err := client.Do(context.Background(), http.MethodPost, "/api/v1/manifests", map[string]string{"name": "fixture"}, nil)
				if err == nil || !strings.Contains(err.Error(), "do not follow redirects") || strings.Contains(err.Error(), "redirect-secret-fixture") {
					t.Fatalf("expected safe redirect refusal, got %v", err)
				}
				wantMutations := int32(0)
				if phase == "mutation" {
					wantMutations = 1
				}
				if redirected.Load() != 0 || mutations.Load() != wantMutations {
					t.Error("session request was redirected or mutation was replayed")
				}
				if client.HTTPClient != originalClient || originalClient.CheckRedirect != nil {
					t.Error("call changed the shared HTTP client's redirect policy")
				}
			})
		}
	}
}

func TestSessionCSRFDoesNotRetryRejectedMutation(t *testing.T) {
	var reads, mutations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/session" {
			reads.Add(1)
			writeJSON(t, w, 200, map[string]any{"authenticated": true, "csrf_token": "csrf_fixture"})
			return
		}
		mutations.Add(1)
		writeJSON(t, w, http.StatusForbidden, map[string]string{"error": "csrf.token_mismatch"})
	}))
	defer server.Close()
	err := NewClient(server.URL, "ys_session_fixture").Do(context.Background(), http.MethodPost, "/api/v1/manifests", nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 403 || apiErr.Detail != "csrf.token_mismatch" {
		t.Fatalf("original authorization/CSRF rejection was lost: %v", err)
	}
	if reads.Load() != 1 || mutations.Load() != 1 {
		t.Error("server rejection must not trigger refresh or mutation retry")
	}
}

func TestSessionCSRFPreservesContextCancellation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(500)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := NewClient(server.URL, "ys_session_fixture").Do(ctx, http.MethodPost, "/api/v1/manifests", nil, nil)
	if !errors.Is(err, context.Canceled) || calls.Load() != 0 {
		t.Fatalf("canceled request must stop before mutation: %v", err)
	}
}
