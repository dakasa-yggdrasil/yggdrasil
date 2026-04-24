package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

// runDescribe fetches one manifest and renders the full document as
// YAML. Convenience on top of `get <kind> <name> -o yaml` — an adopter
// who just wants to inspect something does not have to remember the
// format flag.
func runDescribe(args []string) error {
	fs := flag.NewFlagSet("describe", flag.ContinueOnError)
	namespace := fs.String("namespace", "", "namespace (required when ambiguous)")
	fs.StringVar(namespace, "n", "", "short for --namespace")
	server := fs.String("server", "", "override context server URL")
	token := fs.String("token", "", "override context bearer token")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `yggdrasil describe — print one manifest as YAML

Usage:
  yggdrasil describe <kind> <name> [-n <namespace>]

Flags:
`)
		fs.PrintDefaults()
	}

	var positional []string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			rest = append(rest, a)
			if i+1 < len(args) && !strings.Contains(a, "=") && !strings.HasPrefix(args[i+1], "-") {
				rest = append(rest, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, a)
	}
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if len(positional) < 2 {
		fs.Usage()
		return fmt.Errorf("kind and name are required")
	}
	kind, name := positional[0], positional[1]

	client, err := corecli.FromContext(*server, *token)
	if err != nil {
		return err
	}

	items, err := client.ListManifests(context.Background(), kind, *namespace, name, false)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("no %s found with name %q", kind, name)
	}
	if len(items) > 1 {
		return fmt.Errorf("ambiguous: %d matches for %s/%s (use -n to disambiguate)", len(items), kind, name)
	}

	encoded, err := yaml.Marshal(items[0])
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, string(encoded))
	return nil
}
