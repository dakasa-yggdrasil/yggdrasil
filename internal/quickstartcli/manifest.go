package quickstartcli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sigs.k8s.io/yaml"
)

// DefaultManifestPath is the conventional location of the quickstart
// manifest inside an integration repository. Override via the ":path"
// suffix of repo_ref ("owner/repo:custom/path.yaml").
const DefaultManifestPath = "yggdrasil-quickstart.yaml"

// RepoRef is the parsed form of a repo_ref string ("owner/repo[@ref][:path]").
type RepoRef struct {
	Owner string
	Repo  string
	Ref   string
	Path  string
}

func (r RepoRef) String() string {
	out := r.Owner + "/" + r.Repo
	if r.Ref != "" && r.Ref != "main" {
		out += "@" + r.Ref
	}
	if r.Path != "" && r.Path != DefaultManifestPath {
		out += ":" + r.Path
	}
	return out
}

// ParseRepoRef accepts "owner/repo[@ref][:path]" and applies defaults
// (ref=main, path=yggdrasil-quickstart.yaml). Mirrors the server-side parser
// so that the CLI's representation matches what the install endpoint sees.
func ParseRepoRef(raw string) (RepoRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RepoRef{}, fmt.Errorf("repo_ref is required")
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

func fetchRawManifest(ctx context.Context, ref RepoRef) ([]byte, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", ref.Owner, ref.Repo, ref.Ref, ref.Path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "yggdrasil-cli/quickstart")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
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
