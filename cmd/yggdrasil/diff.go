package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

// runDiff implements `yggdrasil diff -f <file>`: show how the desired
// manifest(s) in a file differ from the active versions on the server.
// The core has no server-side diff/validate for manifests, so this is a
// client-side spec comparison — honest about what it can and can't see.
func runDiff(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	file := fs.String("f", "", "path to a manifest file (use '-' for stdin)")
	server := fs.String("server", "", "override the active context's server URL")
	token := fs.String("token", "", "override the active context's bearer token")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("-f is required")
	}

	raw, err := readManifestSource(*file)
	if err != nil {
		return err
	}
	docs, err := splitYAMLDocuments(raw)
	if err != nil {
		return err
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
		name, _ := payload["name"].(string)
		namespace, _ := payload["namespace"].(string)
		d, isNew, err := diffManifest(ctx, client, kind, namespace, name, payload["spec"])
		if err != nil {
			return err
		}
		switch {
		case isNew:
			fmt.Printf("+ %s %s/%s (new — no active version on server)\n", kind, namespace, name)
		case d == "":
			fmt.Printf("= %s %s/%s (no changes)\n", kind, namespace, name)
		default:
			fmt.Printf("~ %s %s/%s\n%s\n", kind, namespace, name, d)
		}
	}
	return nil
}

// diffManifest compares a desired spec against the active version on the
// server. isNew is true when no active version exists yet.
func diffManifest(ctx context.Context, client *corecli.Client, kind, namespace, name string, desiredSpec any) (diff string, isNew bool, err error) {
	desiredYAML, err := specToYAML(desiredSpec)
	if err != nil {
		return "", false, err
	}
	items, err := client.ListManifests(ctx, kind, namespace, name, true)
	if err != nil {
		return "", false, err
	}
	if len(items) == 0 {
		return "", true, nil
	}
	currentYAML, err := specToYAML(items[0]["spec"])
	if err != nil {
		return "", false, err
	}
	return lineDiff(splitLines(currentYAML), splitLines(desiredYAML)), false, nil
}

// specToYAML normalises any spec (json.RawMessage or decoded map) into
// canonical YAML — routed through JSON so key order and types match,
// preventing phantom diffs from cosmetic ordering.
func specToYAML(spec any) (string, error) {
	jb, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	var v any
	if err := json.Unmarshal(jb, &v); err != nil {
		return "", err
	}
	yb, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(yb), nil
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// lineDiff is a positional line diff: unchanged lines get a " " prefix,
// removals "-", additions "+". It is intentionally simple (no LCS) —
// manifests are edited in place, so positional comparison reads well and
// avoids a diff-library dependency. Returns "" when the inputs match.
func lineDiff(oldLines, newLines []string) string {
	if len(oldLines) == len(newLines) {
		identical := true
		for i := range oldLines {
			if oldLines[i] != newLines[i] {
				identical = false
				break
			}
		}
		if identical {
			return ""
		}
	}
	n := len(oldLines)
	if len(newLines) > n {
		n = len(newLines)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		var o, nw string
		hasO := i < len(oldLines)
		hasN := i < len(newLines)
		if hasO {
			o = oldLines[i]
		}
		if hasN {
			nw = newLines[i]
		}
		if hasO && hasN && o == nw {
			b.WriteString(" " + o + "\n")
			continue
		}
		if hasO {
			b.WriteString("-" + o + "\n")
		}
		if hasN {
			b.WriteString("+" + nw + "\n")
		}
	}
	return b.String()
}
