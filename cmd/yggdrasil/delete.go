package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

// confirmYes reads one line and returns true only for an explicit yes.
// Default (empty / anything else) is NO — destructive ops fail safe.
func confirmYes(r io.Reader) bool {
	s := bufio.NewScanner(r)
	if !s.Scan() {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(s.Text())) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// runDelete implements `yggdrasil delete <kind> <name>`. The core deletes
// by manifest id, so we resolve (kind, namespace, name) → id via a list
// first. Hard by default (mirrors the core); --soft flips active=false
// and keeps history. Prompts unless --yes.
func runDelete(args []string) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	namespace := fs.String("namespace", "global", "manifest namespace")
	fs.StringVar(namespace, "n", "global", "short for --namespace")
	soft := fs.Bool("soft", false, "soft-delete (flip active=false, keep history) instead of hard delete")
	yes := fs.Bool("yes", false, "skip the interactive confirmation")
	server := fs.String("server", "", "override the active context's server URL")
	token := fs.String("token", "", "override the active context's bearer token")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `yggdrasil delete — remove a manifest

Usage:
  yggdrasil delete <kind> <name> [-n <ns>]   hard-delete (default)
  yggdrasil delete <kind> <name> --soft      keep history, flip active=false
  yggdrasil delete <kind> <name> --yes       skip the confirmation prompt

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

	client, err := clientFromFlags(*server, *token)
	if err != nil {
		return err
	}
	ctx := context.Background()

	id, err := resolveManifestID(ctx, client, kind, *namespace, name)
	if err != nil {
		return err
	}

	mode := "hard"
	if *soft {
		mode = "soft"
	}
	if !*yes {
		fmt.Printf("%s-delete %s %s/%s? This cannot be undone. [y/N] ", mode, kind, *namespace, name)
		if !confirmYes(os.Stdin) {
			fmt.Println("aborted")
			return nil
		}
	}

	resp, err := client.DeleteManifest(ctx, id, *soft)
	if err != nil {
		return err
	}
	if absent, _ := resp["already_absent"].(bool); absent {
		fmt.Printf("already absent: %s %s/%s\n", kind, *namespace, name)
		return nil
	}
	fmt.Printf("✓ %s-deleted %s %s/%s\n", mode, kind, *namespace, name)
	return nil
}

// resolveManifestID finds the manifest record id for a (kind, ns, name).
// ListManifests returns every version ordered version DESC, so the first
// row is the latest — and a delete by any version's id covers the whole
// (kind, namespace, name) group server-side.
func resolveManifestID(ctx context.Context, client *corecli.Client, kind, namespace, name string) (string, error) {
	items, err := client.ListManifests(ctx, kind, namespace, name, false)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "", fmt.Errorf("no %s named %q in namespace %q", kind, name, namespace)
	}
	id, ok := nestedString(items[0], "id")
	if !ok || id == "" {
		return "", fmt.Errorf("could not resolve manifest id for %s %s/%s", kind, namespace, name)
	}
	return id, nil
}
