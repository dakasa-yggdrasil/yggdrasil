package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

// runGet implements `yggdrasil get <kind> [name]`.
//
// Without name: lists all manifests of the kind (filtered by namespace
// when --namespace is set).
//
// With name: fetches that one manifest (the "describe" subcommand is
// a thin wrapper that forces full YAML output for convenience).
//
// Output defaults to a table, matching kubectl's ergonomics. -o yaml
// or -o json returns the raw server response for piping.
func runGet(args []string) error {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	namespace := fs.String("namespace", "", "filter by namespace")
	output := fs.String("o", "table", "output format: table | yaml | json")
	activeOnly := fs.Bool("active-only", false, "only return manifests whose metadata.active is true")
	server := fs.String("server", "", "override context server URL")
	token := fs.String("token", "", "override context bearer token")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `yggdrasil get — list or fetch manifests

Usage:
  yggdrasil get <kind>                   list all manifests of a kind
  yggdrasil get <kind> <name>            fetch one manifest by name
  yggdrasil get <kind> -n <ns>           scope to a namespace

Flags:
`)
		fs.PrintDefaults()
	}

	kind, name, flagArgs := splitGetPositionals(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if kind == "" {
		fs.Usage()
		return fmt.Errorf("kind is required")
	}

	client, err := corecli.FromContext(*server, *token)
	if err != nil {
		return err
	}

	items, err := client.ListManifests(context.Background(), kind, *namespace, name, *activeOnly)
	if err != nil {
		return err
	}

	return renderManifests(items, *output)
}

func splitGetPositionals(args []string) (kind string, name string, rest []string) {
	var positional []string
	rest = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			rest = append(rest, a)
			// Simple 2-arg flag handling: `-n ns` vs `-n=ns`. Any flag
			// whose next arg does not start with '-' and is not a known
			// positional slot is consumed as its value.
			if i+1 < len(args) && !strings.Contains(a, "=") && !strings.HasPrefix(args[i+1], "-") {
				rest = append(rest, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, a)
	}
	if len(positional) > 0 {
		kind = positional[0]
	}
	if len(positional) > 1 {
		name = positional[1]
	}
	return kind, name, rest
}

func renderManifests(items []map[string]any, format string) error {
	switch strings.ToLower(format) {
	case "json":
		encoded, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(encoded))
		return nil
	case "yaml":
		encoded, err := yaml.Marshal(items)
		if err != nil {
			return err
		}
		fmt.Println(string(encoded))
		return nil
	case "table", "":
		return renderManifestTable(items)
	default:
		return fmt.Errorf("unknown output format %q (valid: table, yaml, json)", format)
	}
}

func renderManifestTable(items []map[string]any) error {
	if len(items) == 0 {
		fmt.Println("no manifests found")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "KIND\tNAMESPACE\tNAME\tVERSION\tACTIVE")
	sort.Slice(items, func(i, j int) bool {
		ni, _ := nestedString(items[i], "metadata", "name")
		nj, _ := nestedString(items[j], "metadata", "name")
		return ni < nj
	})
	for _, m := range items {
		kind, _ := m["kind"].(string)
		name, _ := nestedString(m, "metadata", "name")
		namespace, _ := nestedString(m, "metadata", "namespace")
		version, _ := m["version"].(float64)
		activeVal, _ := nestedBool(m, "metadata", "active")
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%t\n", kind, namespace, name, int(version), activeVal)
	}
	return w.Flush()
}

func nestedBool(m map[string]any, keys ...string) (bool, bool) {
	var current any = m
	for _, key := range keys {
		mm, ok := current.(map[string]any)
		if !ok {
			return false, false
		}
		current = mm[key]
	}
	b, ok := current.(bool)
	return b, ok
}
