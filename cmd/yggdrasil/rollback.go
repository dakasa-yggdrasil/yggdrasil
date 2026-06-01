package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
)

// manifestVersion reads the top-level integer version off a manifest map
// (JSON numbers arrive as float64).
func manifestVersion(m map[string]any) (int, bool) {
	if v, ok := m["version"].(float64); ok {
		return int(v), true
	}
	return 0, false
}

// runRollback implements `yggdrasil rollback <kind> <name> --to <version>`.
// The core has no rollback endpoint, but manifests are versioned and the
// list returns every version with its spec — so a rollback is just
// re-applying the target version's spec as a fresh version. History is
// preserved (you can roll forward again).
func runRollback(args []string) error {
	fs := flag.NewFlagSet("rollback", flag.ContinueOnError)
	namespace := fs.String("namespace", "global", "manifest namespace")
	fs.StringVar(namespace, "n", "global", "short for --namespace")
	to := fs.Int("to", 0, "the manifest version to roll back to (required)")
	yes := fs.Bool("yes", false, "skip the interactive confirmation")
	server := fs.String("server", "", "override the active context's server URL")
	token := fs.String("token", "", "override the active context's bearer token")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `yggdrasil rollback — re-apply an older manifest version as a new version

Usage:
  yggdrasil rollback <kind> <name> --to <version> [-n <ns>]

Flags:
`)
		fs.PrintDefaults()
	}

	kind, name, flagArgs := splitGetPositionals(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if kind == "" || name == "" {
		fs.Usage()
		return errors.New("both <kind> and <name> are required")
	}
	if *to < 1 {
		return errors.New("--to <version> is required (a positive manifest version)")
	}

	client, err := clientFromFlags(*server, *token)
	if err != nil {
		return err
	}
	ctx := context.Background()

	items, err := client.ListManifests(ctx, kind, *namespace, name, false)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("no %s named %q in namespace %q", kind, name, *namespace)
	}

	var target map[string]any
	latest := 0
	for _, it := range items {
		if v, ok := manifestVersion(it); ok {
			if v > latest {
				latest = v
			}
			if v == *to {
				target = it
			}
		}
	}
	if target == nil {
		return fmt.Errorf("version %d of %s %s/%s not found (latest is %d)", *to, kind, *namespace, name, latest)
	}
	if _, ok := target["spec"]; !ok {
		return fmt.Errorf("version %d of %s %s/%s has no spec to restore", *to, kind, *namespace, name)
	}

	if !*yes {
		fmt.Printf("roll back %s %s/%s from v%d to v%d? a new version will be created. [y/N] ", kind, *namespace, name, latest, *to)
		if !confirmYes(os.Stdin) {
			fmt.Println("aborted")
			return nil
		}
	}

	payload := map[string]any{
		"name":      name,
		"namespace": *namespace,
		"spec":      target["spec"],
	}
	created, err := client.CreateManifest(ctx, kind, payload)
	if err != nil {
		return fmt.Errorf("re-apply v%d: %w", *to, err)
	}
	newVer, _ := manifestVersion(created)
	fmt.Printf("✓ rolled back %s %s/%s to the contents of v%d (new version %d)\n", kind, *namespace, name, *to, newVer)
	return nil
}
