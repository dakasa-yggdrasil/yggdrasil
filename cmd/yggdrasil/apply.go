package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

// runApply implements `yggdrasil apply -f <file>`. Reads a manifest
// document (YAML or JSON) and POSTs it to /api/v1/<kind>s. Supports
// multiple documents in one YAML file (separated by `---`) so an
// adopter can apply a whole bundle without per-file looping.
//
// The subcommand is how adopters put anything into the engine without
// curl or kubectl-like manifest editors. It is intentionally thin:
// the server is authoritative on schema, the CLI only shapes the HTTP
// call and prints the response.
func runApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	file := fs.String("f", "", "path to a manifest file (use '-' for stdin)")
	server := fs.String("server", "", "override the active context's server URL")
	token := fs.String("token", "", "override the active context's bearer token")
	dryRun := fs.Bool("dry-run", false, "show the diff vs the active version without applying")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `yggdrasil apply — create a new manifest version from a file

Usage:
  yggdrasil apply -f my-workflow.yaml
  yggdrasil apply -f - < bundle.yaml

Multiple documents may be separated by '---' (standard YAML).

Flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		fs.Usage()
		return fmt.Errorf("-f is required")
	}

	raw, err := readManifestSource(*file)
	if err != nil {
		return err
	}

	docs, err := splitYAMLDocuments(raw)
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return fmt.Errorf("no manifest documents found in %s", *file)
	}

	client, err := corecli.FromContext(*server, *token)
	if err != nil {
		return err
	}
	ctx := context.Background()

	for _, doc := range docs {
		kind, payload, err := extractKindAndPayload(doc)
		if err != nil {
			return err
		}
		if *dryRun {
			name, _ := payload["name"].(string)
			namespace, _ := payload["namespace"].(string)
			d, isNew, derr := diffManifest(ctx, client, kind, namespace, name, payload["spec"])
			if derr != nil {
				return derr
			}
			switch {
			case isNew:
				fmt.Printf("+ %s %s/%s (new — would be created)\n", kind, namespace, name)
			case d == "":
				fmt.Printf("= %s %s/%s (no changes)\n", kind, namespace, name)
			default:
				fmt.Printf("~ %s %s/%s (dry-run, not applied)\n%s\n", kind, namespace, name, d)
			}
			continue
		}
		created, err := client.CreateManifest(ctx, kind, payload)
		if err != nil {
			return err
		}
		name, _ := nestedString(created, "metadata", "name")
		namespace, _ := nestedString(created, "metadata", "namespace")
		version, _ := created["version"].(float64)
		fmt.Printf("created %s %s/%s (version %d)\n", kind, namespace, name, int(version))
	}
	return nil
}

func readManifestSource(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

// splitYAMLDocuments respects the standard "---" document delimiter so
// a single file can apply a whole bundle. Empty documents are
// filtered out so trailing delimiters (a common editor habit) don't
// fail.
func splitYAMLDocuments(raw []byte) ([][]byte, error) {
	text := string(raw)
	parts := strings.Split(text, "\n---")
	out := make([][]byte, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		trimmed = strings.TrimPrefix(trimmed, "---")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}
		out = append(out, []byte(trimmed))
	}
	return out, nil
}

// extractKindAndPayload converts the raw YAML document into (kind,
// consoleCreateManifestRequest-equivalent payload). The server accepts
// {name, namespace, description, labels, spec} so we map the manifest
// envelope down to that shape.
func extractKindAndPayload(raw []byte) (string, map[string]any, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return "", nil, fmt.Errorf("parse manifest YAML: %w", err)
	}
	kind, _ := doc["kind"].(string)
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		return "", nil, fmt.Errorf("manifest has no kind")
	}
	metadata, _ := doc["metadata"].(map[string]any)
	if metadata == nil {
		return "", nil, fmt.Errorf("manifest has no metadata")
	}
	spec := doc["spec"]
	if spec == nil {
		return "", nil, fmt.Errorf("manifest has no spec")
	}

	name, _ := metadata["name"].(string)
	namespace, _ := metadata["namespace"].(string)
	description, _ := metadata["description"].(string)
	labels, _ := metadata["labels"].(map[string]any)

	// Re-marshal spec to raw JSON the server can decode into
	// json.RawMessage without losing nested types.
	specRaw, err := json.Marshal(spec)
	if err != nil {
		return "", nil, fmt.Errorf("marshal spec: %w", err)
	}

	return kind, map[string]any{
		"name":        name,
		"namespace":   namespace,
		"description": description,
		"labels":      stringifyLabels(labels),
		"spec":        json.RawMessage(specRaw),
	}, nil
}

func stringifyLabels(in map[string]any) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		if s, ok := v.(string); ok {
			out[k] = s
		} else {
			out[k] = fmt.Sprintf("%v", v)
		}
	}
	return out
}

func nestedString(m map[string]any, keys ...string) (string, bool) {
	var current any = m
	for _, key := range keys {
		mm, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		current = mm[key]
	}
	s, ok := current.(string)
	return s, ok
}
