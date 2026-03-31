package httpapi

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dakasa-co/yggdrasil-core/model"
	"github.com/dakasa-co/yggdrasil-core/repository"
)

type authLoginResponse struct {
	Collaborator model.Collaborator `json:"collaborator"`
	Session      model.AuthSession  `json:"session"`
	Token        string             `json:"token"`
}

type authThirdPartyLoginResponse struct {
	Collaborator model.Collaborator       `json:"collaborator"`
	Identity     model.ThirdPartyIdentity `json:"identity"`
	Session      model.AuthSession        `json:"session"`
	Token        string                   `json:"token"`
}

type thirdPartyIdentitiesResponse struct {
	Identities []model.ThirdPartyIdentity `json:"identities"`
}

func (s *Server) handleAuthPasswordUpsert(w http.ResponseWriter, r *http.Request) {
	var req model.UpsertPasswordCredentialRequest
	if err := decodeJSON(r, &req); err != nil {
		writeMappedError(w, err)
		return
	}

	credential, collaborator, err := repository.UpsertPasswordCredential(r.Context(), s.db, req)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"credential":   credential,
		"collaborator": collaborator,
	})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var req model.LoginWithPasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeMappedError(w, err)
		return
	}

	req.Metadata = mergeAuthMetadata(req.Metadata, r)
	collaborator, session, token, err := repository.AuthenticateWithPassword(
		r.Context(),
		s.db,
		req,
		authSessionTTL(),
	)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	writeAuthCookie(w, token, session.ExpiresAt)
	writeJSON(w, http.StatusOK, authLoginResponse{
		Collaborator: collaborator,
		Session:      session,
		Token:        token,
	})
}

func (s *Server) handleAuthThirdPartyLogin(w http.ResponseWriter, r *http.Request) {
	var req model.LoginWithThirdPartyIdentityRequest
	if err := decodeJSON(r, &req); err != nil {
		writeMappedError(w, err)
		return
	}

	req.SessionMetadata = mergeAuthMetadata(req.SessionMetadata, r)
	collaborator, identity, session, token, err := repository.AuthenticateWithThirdPartyIdentity(
		r.Context(),
		s.db,
		req,
		authSessionTTL(),
	)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	writeAuthCookie(w, token, session.ExpiresAt)
	writeJSON(w, http.StatusOK, authThirdPartyLoginResponse{
		Collaborator: collaborator,
		Identity:     identity,
		Session:      session,
		Token:        token,
	})
}

func (s *Server) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	token, ok := extractAuthToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, model.AuthSessionEnvelope{Authenticated: false})
		return
	}

	session, collaborator, err := repository.ResolveAuthSession(r.Context(), s.db, token)
	if err != nil {
		if isAuthUnauthorizedError(err) {
			clearAuthCookie(w)
			writeJSON(w, http.StatusUnauthorized, model.AuthSessionEnvelope{Authenticated: false})
			return
		}
		writeMappedError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, model.AuthSessionEnvelope{
		Authenticated: true,
		Collaborator:  &collaborator,
		Session:       &session,
	})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	token, ok := extractAuthToken(r)
	if !ok {
		clearAuthCookie(w)
		writeJSON(w, http.StatusOK, model.AuthSessionEnvelope{Authenticated: false})
		return
	}

	session, err := repository.RevokeAuthSession(r.Context(), s.db, token)
	if err != nil {
		if isAuthUnauthorizedError(err) {
			clearAuthCookie(w)
			writeJSON(w, http.StatusOK, model.AuthSessionEnvelope{Authenticated: false})
			return
		}
		writeMappedError(w, err)
		return
	}

	clearAuthCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": false,
		"session":       session,
	})
}

func (s *Server) handleThirdPartyIdentityList(w http.ResponseWriter, r *http.Request) {
	identities, err := repository.ListThirdPartyIdentities(r.Context(), s.db, model.ListThirdPartyIdentitiesRequest{
		CollaboratorID: queryString(r, "collaborator_id"),
		Provider:       queryString(r, "provider"),
		Status:         queryString(r, "status"),
	})
	if err != nil {
		writeMappedError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, thirdPartyIdentitiesResponse{Identities: identities})
}

func (s *Server) handleThirdPartyIdentityUpsert(w http.ResponseWriter, r *http.Request) {
	var req model.UpsertThirdPartyIdentityRequest
	if err := decodeJSON(r, &req); err != nil {
		writeMappedError(w, err)
		return
	}

	identity, collaborator, err := repository.UpsertThirdPartyIdentity(r.Context(), s.db, req)
	if err != nil {
		writeMappedError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"identity":     identity,
		"collaborator": collaborator,
	})
}

func (s *Server) handleThirdPartyIdentityDelete(w http.ResponseWriter, r *http.Request) {
	identity, collaborator, err := repository.DeleteThirdPartyIdentity(r.Context(), s.db, model.DeleteThirdPartyIdentityRequest{
		Provider: r.PathValue("provider"),
		Subject:  r.PathValue("subject"),
	})
	if err != nil {
		writeMappedError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"identity":     identity,
		"collaborator": collaborator,
	})
}

func writeAuthCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     authSessionCookieName(),
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   authSessionCookieSecure(),
		Domain:   authSessionCookieDomain(),
	})
}

func clearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     authSessionCookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   authSessionCookieSecure(),
		Domain:   authSessionCookieDomain(),
	})
}

func extractAuthToken(r *http.Request) (string, bool) {
	if cookie, err := r.Cookie(authSessionCookieName()); err == nil {
		if value := strings.TrimSpace(cookie.Value); value != "" {
			return value, true
		}
	}

	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		if token := strings.TrimSpace(authHeader[7:]); token != "" {
			return token, true
		}
	}

	if token := strings.TrimSpace(r.Header.Get("X-Session-Token")); token != "" {
		return token, true
	}

	return "", false
}

func mergeAuthMetadata(input map[string]any, r *http.Request) map[string]any {
	output := cloneAnyMap(input)
	if output == nil {
		output = map[string]any{}
	}

	if surface := strings.TrimSpace(r.Header.Get("X-Yggdrasil-Surface")); surface != "" {
		output["surface"] = surface
	}
	if userAgent := strings.TrimSpace(r.UserAgent()); userAgent != "" {
		output["user_agent"] = userAgent
	}
	if remoteAddr := strings.TrimSpace(r.RemoteAddr); remoteAddr != "" {
		output["remote_addr"] = remoteAddr
	}

	return output
}

func authSessionCookieName() string {
	if value := strings.TrimSpace(os.Getenv("AUTH_SESSION_COOKIE_NAME")); value != "" {
		return value
	}
	return "yggdrasil_session"
}

func authSessionCookieDomain() string {
	return strings.TrimSpace(os.Getenv("AUTH_SESSION_COOKIE_DOMAIN"))
}

func authSessionCookieSecure() bool {
	value := strings.TrimSpace(os.Getenv("AUTH_SESSION_COOKIE_SECURE"))
	if value == "" {
		return false
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false
	}
	return parsed
}

func authSessionTTL() time.Duration {
	value := strings.TrimSpace(os.Getenv("AUTH_SESSION_TTL_HOURS"))
	if value == "" {
		return 30 * 24 * time.Hour
	}

	hours, err := strconv.Atoi(value)
	if err != nil || hours <= 0 {
		return 30 * 24 * time.Hour
	}
	return time.Duration(hours) * time.Hour
}

func isAuthUnauthorizedError(err error) bool {
	switch {
	case err == nil:
		return false
	case err == repository.ErrAuthInvalidCredentials,
		err == repository.ErrAuthSessionNotFound,
		err == repository.ErrAuthSessionExpired,
		err == repository.ErrPasswordCredentialNotFound,
		err == repository.ErrCollaboratorNotFound:
		return true
	default:
		return false
	}
}
