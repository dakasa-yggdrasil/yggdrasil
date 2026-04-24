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

	"sigs.k8s.io/yaml"
)

// DefaultManifestPath is the conventional location of the quickstart
// manifest inside an integration repository. Override via the ":path"
// suffix of repo_ref ("owner/repo:custom/path.yaml").
const DefaultManifestPath = "yggdrasil-quickstart.yaml"

// RepoRef is the parsed form of a repo_ref string. Two forms are
// supported:
//
//   - GitHub: "owner/repo[@ref][:path]" — Owner/Repo/Ref/Path set, OCI nil.
//   - OCI:    "oci://<registry>/<path>[:<tag>]" — OCI set, Owner empty.
//
// Downstream fetchers branch on OCI != nil to pick the right
// transport. Keeping a single RepoRef type (rather than an interface)
// means the rest of the CLI doesn't change — server payloads still
// receive a canonical String() representation.
type RepoRef struct {
	Owner string
	Repo  string
	Ref   string
	Path  string

	OCI *OCIRef
}

func (r RepoRef) String() string {
	if r.OCI != nil {
		return r.OCI.String()
	}
	out := r.Owner + "/" + r.Repo
	if r.Ref != "" && r.Ref != "main" {
		out += "@" + r.Ref
	}
	if r.Path != "" && r.Path != DefaultManifestPath {
		out += ":" + r.Path
	}
	return out
}

// ParseRepoRef accepts either:
//
//   - "oci://<registry>/<path>[:<tag>]"
//   - "owner/repo[@ref][:path]" (GitHub)
//
// and applies defaults (ref=main, path=yggdrasil-quickstart.yaml for
// GitHub; tag=latest for OCI). Mirrors the server-side parser so the
// CLI's representation matches what the install endpoint sees.
func ParseRepoRef(raw string) (RepoRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RepoRef{}, fmt.Errorf("repo_ref is required")
	}
	if IsOCIRef(raw) {
		ref, err := ParseOCIRef(raw)
		if err != nil {
			return RepoRef{}, err
		}
		return RepoRef{OCI: &ref}, nil
	}

	rest := raw
	var path string
	if i := strings.Index(rest, ":"); i >= 0 {
		path = strings.TrimSpace(rest[i+1:])
		rest = rest[:i]
	}
	var ref string
	if i := strings.Index(rest, "@"); i >= 0 {
		ref = strings.TrimSpace(rest[i+1:])
		rest = rest[:i]
	}
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return RepoRef{}, fmt.Errorf("repo_ref %q must look like owner/repo[@ref][:path]", raw)
	}
	if ref == "" {
		ref = "main"
	}
	if path == "" {
		path = DefaultManifestPath
	}
	return RepoRef{
		Owner: strings.TrimSpace(parts[0]),
		Repo:  strings.TrimSpace(parts[1]),
		Ref:   ref,
		Path:  path,
	}, nil
}

// FetchManifest downloads the quickstart manifest from raw.githubusercontent.com
// and decodes it (YAML or JSON). The CLI fetches it locally so the TUI can
// render forms BEFORE round-tripping to the server — the server fetches the
// same file again at install time, which is fine because manifests are small.
func FetchManifest(ctx context.Context, ref RepoRef) (QuickstartDocument, []byte, error) {
	raw, err := fetchRawManifest(ctx, ref)
	if err != nil {
		return QuickstartDocument{}, nil, err
	}
	doc, err := DecodeManifest(raw)
	if err != nil {
		return QuickstartDocument{}, raw, err
	}
	return doc, raw, nil
}

// fetchRawManifest routes to the right transport based on the parsed
// RepoRef: OCI registries use the Distribution v2 API; GitHub refs
// hit the contents API when a token is set (so private repos work)
// and fall back to raw.githubusercontent.com otherwise.
func fetchRawManifest(ctx context.Context, ref RepoRef) ([]byte, error) {
	if ref.OCI != nil {
		return FetchOCIManifest(ctx, *ref.OCI)
	}
	token := strings.TrimSpace(os.Getenv("YGGDRASIL_GITHUB_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	}

	var url string
	headers := map[string]string{"User-Agent": "yggdrasil-cli/quickstart"}
	if token != "" {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", ref.Owner, ref.Repo, ref.Path, ref.Ref)
		headers["Authorization"] = "Bearer " + token
		headers["Accept"] = "application/vnd.github.raw"
	} else {
		url = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", ref.Owner, ref.Repo, ref.Ref, ref.Path)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch %s: HTTP %d %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return body, nil
}

// DecodeManifest accepts YAML or JSON bytes and returns a typed
// QuickstartDocument. JSON is detected by a leading '{'.
func DecodeManifest(raw []byte) (QuickstartDocument, error) {
	trimmed := bytesTrimSpace(raw)
	var jsonBytes []byte
	if len(trimmed) > 0 && trimmed[0] == '{' {
		jsonBytes = raw
	} else {
		converted, err := yaml.YAMLToJSON(raw)
		if err != nil {
			return QuickstartDocument{}, fmt.Errorf("convert YAML to JSON: %w", err)
		}
		jsonBytes = converted
	}
	var doc QuickstartDocument
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		return QuickstartDocument{}, fmt.Errorf("decode manifest: %w", err)
	}
	if doc.Kind != "integration_quickstart" {
		return QuickstartDocument{}, fmt.Errorf("expected kind=integration_quickstart, got %q", doc.Kind)
	}
	return doc, nil
}

func bytesTrimSpace(b []byte) []byte {
	i, j := 0, len(b)
	for i < j {
		c := b[i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		i++
	}
	for j > i {
		c := b[j-1]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		j--
	}
	return b[i:j]
}
