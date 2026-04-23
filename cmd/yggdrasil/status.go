package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

// runStatus prints which yggdrasil-core the CLI is currently aimed
// at, whether it's reachable, and a quick summary of the active
// context. Equivalent of `kubectl cluster-info` for the engine.
func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	server := fs.String("server", "", "override context server URL")
	token := fs.String("token", "", "override context bearer token")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `yggdrasil status — show the active context and a server health check

Usage:
  yggdrasil status

Flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, path, err := corecli.Load()
	if err != nil {
		return err
	}
	active, name := cfg.Active()

	fmt.Printf("config:  %s\n", path)
	if active == nil {
		fmt.Println("context: (none — run `yggdrasil init` or `yggdrasil login`)")
		return nil
	}
	fmt.Printf("context: %s\n", name)
	fmt.Printf("server:  %s\n", firstNonEmpty(*server, active.Server))
	if active.Collaborator != "" {
		fmt.Printf("user:    %s\n", active.Collaborator)
	}

	client, err := corecli.FromContext(*server, *token)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Healthz(ctx); err != nil {
		fmt.Printf("health:  unreachable (%v)\n", err)
		return err
	}
	if err := client.Readyz(ctx); err != nil {
		fmt.Printf("ready:   not ready (%v)\n", err)
		return err
	}
	fmt.Println("health:  ok")
	fmt.Println("ready:   ok")
	return nil
}
