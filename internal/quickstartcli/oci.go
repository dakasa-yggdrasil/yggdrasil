package quickstartcli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OCI reference scheme: oci://<registry>/<path>[:<tag>]
//
// Example:
//
//	oci://ghcr.io/dakasa-yggdrasil/integration-kubernetes:v1.0.0
//	oci://ghcr.io/dakasa-yggdrasil/integration-kubernetes        (defaults to :latest)
//
// We talk Distribution Spec v2 directly (no ORAS dep) because we
// only need two read operations: fetch manifest, fetch one blob.
// For registries that require token exchange (GHCR does, even
// anonymously), we handle the Bearer challenge inline.

const (
	ociScheme            = "oci://"
	ociDefaultTag        = "latest"
	ociQuickstartMediaTp = "application/vnd.yggdrasil.quickstart.v1+yaml"

	// ociManifestAccept is the union of manifest media types the
	// registry may return. OCI artifacts use image manifests as the
	// carrier shape, which is what `oras push` produces by default.
	ociManifestAccept = "application/vnd.oci.image.manifest.v1+json," +
		"application/vnd.docker.distribution.manifest.v2+json"
)

// OCIRef is the parsed form of an oci:// reference.
type OCIRef struct {
	Registry string // e.g. "ghcr.io"
	Path     string // e.g. "dakasa-yggdrasil/integration-kubernetes"
	Tag      string // e.g. "v1.0.0"; defaults to "latest"
}

// String is the canonical oci://... form. The Tag is only included
// when it differs from the default, matching how RepoRef.String works.
func (r OCIRef) String() string {
	out := ociScheme + r.Registry + "/" + r.Path
	if r.Tag != "" && r.Tag != ociDefaultTag {
		out += ":" + r.Tag
	}
	return out
}

// IsOCIRef reports whether raw looks like an oci:// reference.
// Cheap to call so the CLI can branch on form without allocating.
func IsOCIRef(raw string) bool {
	return strings.HasPrefix(strings.TrimSpace(raw), ociScheme)
}

// ParseOCIRef accepts oci://<registry>/<path>[:<tag>] and returns the
// typed form. Path must have at least one slash (registry + namespace
// at minimum); tag defaults to "latest" when absent.
func ParseOCIRef(raw string) (OCIRef, error) {
	raw = strings.TrimSpace(raw)
	if !IsOCIRef(raw) {
		return OCIRef{}, fmt.Errorf("oci_ref %q must start with %s", raw, ociScheme)
	}
	rest := strings.TrimPrefix(raw, ociScheme)

	// Tag separator is the last ":" that is NOT inside a port number
	// of the registry host. A well-formed ref has the tag after the
	// last slash, so we only look right of the final '/'.
	var tag string
	if slash := strings.LastIndex(rest, "/"); slash >= 0 {
		if colon := strings.LastIndex(rest[slash:], ":"); colon >= 0 {
			tag = strings.TrimSpace(rest[slash+colon+1:])
			rest = rest[:slash+colon]
		}
	}

	// Registry is everything before the FIRST slash; path is the
	// remainder. Registries can contain ports (host:5000) which is
	// why we split here and not earlier.
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return OCIRef{}, fmt.Errorf("oci_ref %q is missing the path segment", raw)
	}
	registry := strings.TrimSpace(rest[:slash])
	path := strings.TrimSpace(rest[slash+1:])
	if registry == "" || path == "" {
		return OCIRef{}, fmt.Errorf("oci_ref %q must look like oci://<registry>/<path>[:<tag>]", raw)
	}
	if tag == "" {
		tag = ociDefaultTag
	}
	return OCIRef{Registry: registry, Path: path, Tag: tag}, nil
}

// FetchOCIManifest performs the two-step OCI read (manifest → blob)
// and returns the raw bytes of the first layer matching
// ociQuickstartMediaTp. Falls back to the first layer when no layer
// declares that media type — so integrations published with oras'
// default media types still work without forcing a republish.
func FetchOCIManifest(ctx context.Context, ref OCIRef) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	token, err := fetchOCIBearerToken(ctx, client, ref)
	if err != nil {
		return nil, err
	}

	manifest, err := fetchOCIManifestObject(ctx, client, ref, token)
	if err != nil {
		return nil, err
	}

	digest, err := pickQuickstartLayer(manifest)
	if err != nil {
		return nil, err
	}

	return fetchOCIBlob(ctx, client, ref, token, digest)
}

// fetchOCIBearerToken performs the auth ping against the registry
// and, if it returns a WWW-Authenticate: Bearer challenge, exchanges
// credentials for a scoped token. Anonymous pulls of public repos
// still need this exchange on GHCR.
func fetchOCIBearerToken(ctx context.Context, client *http.Client, ref OCIRef) (string, error) {
	pingURL := fmt.Sprintf("https://%s/v2/", ref.Registry)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pingURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("registry ping %s: %w", pingURL, err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		// Registry allows anonymous access without token exchange.
		return "", nil
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return "", fmt.Errorf("registry ping %s: HTTP %d", pingURL, resp.StatusCode)
	}

	challenge := resp.Header.Get("Www-Authenticate")
	if !strings.HasPrefix(strings.ToLower(challenge), "bearer ") {
		return "", fmt.Errorf("registry %s requires an auth scheme we don't speak: %q", ref.Registry, challenge)
	}
	params := parseBearerChallenge(challenge[len("bearer "):])
	realm := params["realm"]
	if realm == "" {
		return "", fmt.Errorf("registry %s issued a Bearer challenge without a realm", ref.Registry)
	}

	tokenURL := realm
	q := []string{}
	if v := params["service"]; v != "" {
		q = append(q, "service="+v)
	}
	q = append(q, "scope=repository:"+ref.Path+":pull")
	if len(q) > 0 {
		separator := "?"
		if strings.Contains(tokenURL, "?") {
			separator = "&"
		}
		tokenURL += separator + strings.Join(q, "&")
	}

	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", err
	}
	if user, pass := ociCredentials(ref.Registry); pass != "" {
		tokenReq.SetBasicAuth(user, pass)
	}
	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return "", fmt.Errorf("token exchange %s: %w", tokenURL, err)
	}
	defer tokenResp.Body.Close()
	if tokenResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(tokenResp.Body)
		return "", fmt.Errorf("token exchange %s: HTTP %d %s", tokenURL, tokenResp.StatusCode, strings.TrimSpace(string(body)))
	}
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if body.Token != "" {
		return body.Token, nil
	}
	return body.AccessToken, nil
}

// ociCredentials returns the username + password the CLI should use
// against the registry. Order:
//
//   - OCI_USERNAME / OCI_PASSWORD — explicit override.
//   - For ghcr.io: GITHUB_TOKEN (or YGGDRASIL_GITHUB_TOKEN) → "x-access-token" username.
//
// Empty password means anonymous — still works for public repos on
// GHCR, just exchanged for a read-only anonymous token.
func ociCredentials(registry string) (string, string) {
	if user := strings.TrimSpace(os.Getenv("OCI_USERNAME")); user != "" {
		return user, strings.TrimSpace(os.Getenv("OCI_PASSWORD"))
	}
	if strings.EqualFold(registry, "ghcr.io") {
		token := strings.TrimSpace(os.Getenv("YGGDRASIL_GITHUB_TOKEN"))
		if token == "" {
			token = strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
		}
		if token != "" {
			return "x-access-token", token
		}
	}
	return "", ""
}

func parseBearerChallenge(s string) map[string]string {
	out := map[string]string{}
	for _, part := range splitUnquoted(s, ',') {
		part = strings.TrimSpace(part)
		eq := strings.Index(part, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(part[:eq])
		value := strings.Trim(strings.TrimSpace(part[eq+1:]), `"`)
		out[strings.ToLower(key)] = value
	}
	return out
}

// splitUnquoted splits by sep but ignores seps inside "..." regions.
// Bearer challenge values are comma-separated with quoted strings
// that may themselves contain commas.
func splitUnquoted(s string, sep byte) []string {
	out := []string{}
	var current strings.Builder
	inQuotes := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			inQuotes = !inQuotes
			current.WriteByte(c)
			continue
		}
		if c == sep && !inQuotes {
			out = append(out, current.String())
			current.Reset()
			continue
		}
		current.WriteByte(c)
	}
	if current.Len() > 0 {
		out = append(out, current.String())
	}
	return out
}

type ociManifest struct {
	MediaType string     `json:"mediaType"`
	Layers    []ociLayer `json:"layers"`
}

type ociLayer struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

func fetchOCIManifestObject(ctx context.Context, client *http.Client, ref OCIRef, token string) (ociManifest, error) {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", ref.Registry, ref.Path, ref.Tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ociManifest{}, err
	}
	req.Header.Set("Accept", ociManifestAccept)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return ociManifest{}, fmt.Errorf("fetch manifest %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return ociManifest{}, fmt.Errorf("fetch manifest %s: HTTP %d %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var manifest ociManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return ociManifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	return manifest, nil
}

func pickQuickstartLayer(manifest ociManifest) (string, error) {
	if len(manifest.Layers) == 0 {
		return "", fmt.Errorf("oci manifest has zero layers")
	}
	for _, layer := range manifest.Layers {
		if layer.MediaType == ociQuickstartMediaTp {
			return layer.Digest, nil
		}
	}
	// Fallback: some integrations publish the yaml with a generic
	// media type (application/yaml, application/octet-stream, etc.).
	// The first layer is the convention when there's only one.
	return manifest.Layers[0].Digest, nil
}

func fetchOCIBlob(ctx context.Context, client *http.Client, ref OCIRef, token, digest string) ([]byte, error) {
	url := fmt.Sprintf("https://%s/v2/%s/blobs/%s", ref.Registry, ref.Path, digest)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch blob %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch blob %s: HTTP %d %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}
