package corecli

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestLoginMFA_SendsTOTPAndReturnsToken(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"token":        "ys_tok",
			"collaborator": map[string]any{"slug": "alice"},
		})
	})
	res, err := fs.client().LoginMFA(context.Background(), "alice@example.com", "pw", "123456", "")
	if err != nil {
		t.Fatalf("LoginMFA: %v", err)
	}
	if res.Token != "ys_tok" || res.CollaboratorSlug != "alice" || res.MFARequired {
		t.Errorf("unexpected result: %+v", res)
	}
	if fs.last.body["totp_code"] != "123456" {
		t.Errorf("totp_code not sent in body: %+v", fs.last.body)
	}
	if fs.last.method != http.MethodPost || fs.last.path != "/api/v1/auth/login" {
		t.Errorf("wrong request: %s %s", fs.last.method, fs.last.path)
	}
}

func TestLoginMFA_DetectsMFARequired(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		// The core returns 202 + {error: mfa_required, factors} when the
		// password is correct but a second factor is still needed.
		writeJSON(t, w, http.StatusAccepted, map[string]any{
			"error":   "mfa_required",
			"factors": []string{"totp"},
		})
	})
	res, err := fs.client().LoginMFA(context.Background(), "alice", "pw", "", "")
	if err != nil {
		t.Fatalf("LoginMFA: %v", err)
	}
	if !res.MFARequired || res.Token != "" {
		t.Errorf("expected MFARequired with no token, got %+v", res)
	}
	if len(res.Factors) != 1 || res.Factors[0] != "totp" {
		t.Errorf("factors not parsed: %+v", res.Factors)
	}
}

func TestLogin_ErrorsClearlyWhenMFARequired(t *testing.T) {
	fs := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusAccepted, map[string]any{
			"error": "mfa_required", "factors": []string{"totp"},
		})
	})
	_, _, err := fs.client().Login(context.Background(), "alice", "pw")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "mfa") {
		t.Fatalf("expected a clear MFA error from Login, got %v", err)
	}
}
