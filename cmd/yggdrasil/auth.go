package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

// runAuth dispatches `yggdrasil auth ...` subcommands. Today only
// `provider` exists; future additions (sessions, tokens) live under
// the same root so we do not pollute the top-level help.
func runAuth(args []string) error {
	if len(args) == 0 {
		fmt.Println(`yggdrasil auth — manage authentication providers

Usage:
  yggdrasil auth provider list                       list configured OAuth/OIDC providers
  yggdrasil auth provider get <name>                 fetch one provider
  yggdrasil auth provider apply -f <file>            register/update a provider from YAML
  yggdrasil auth provider delete <name>              remove a provider`)
		return nil
	}

	switch args[0] {
	case "provider", "providers":
		return runAuthProvider(args[1:])
	default:
		return fmt.Errorf("unknown auth subcommand: %s", args[0])
	}
}

func runAuthProvider(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: yggdrasil auth provider <list|get|apply|delete> ...")
	}

	switch args[0] {
	case "list":
		return runAuthProviderList(args[1:])
	case "get":
		return runAuthProviderGet(args[1:])
	case "apply":
		return runAuthProviderApply(args[1:])
	case "delete":
		return runAuthProviderDelete(args[1:])
	default:
		return fmt.Errorf("unknown provider subcommand: %s", args[0])
	}
}

func runAuthProviderList(args []string) error {
	fs := flag.NewFlagSet("auth provider list", flag.ContinueOnError)
	server := fs.String("server", "", "override context server URL")
	token := fs.String("token", "", "override context bearer token")
	if err := fs.Parse(args); err != nil {
		return err
	}
	client, err := corecli.FromContext(*server, *token)
	if err != nil {
		return err
	}
	var resp struct {
		Providers []map[string]any `json:"providers"`
	}
	if err := client.Do(context.Background(), http.MethodGet, "/api/v1/auth/providers", nil, &resp); err != nil {
		return err
	}
	if len(resp.Providers) == 0 {
		fmt.Println("no providers configured")
		return nil
	}
	for _, p := range resp.Providers {
		name, _ := p["name"].(string)
		typ, _ := p["type"].(string)
		status, _ := p["status"].(string)
		fmt.Printf("%-24s %-10s %s\n", name, typ, status)
	}
	return nil
}

func runAuthProviderGet(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: yggdrasil auth provider get <name>")
	}
	name := args[0]
	client, err := corecli.FromContext("", "")
	if err != nil {
		return err
	}
	var provider map[string]any
	if err := client.Do(context.Background(), http.MethodGet, "/api/v1/auth/providers/"+name, nil, &provider); err != nil {
		return err
	}
	encoded, err := yaml.Marshal(provider)
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func runAuthProviderApply(args []string) error {
	fs := flag.NewFlagSet("auth provider apply", flag.ContinueOnError)
	file := fs.String("f", "", "path to provider YAML/JSON")
	server := fs.String("server", "", "override context server URL")
	token := fs.String("token", "", "override context bearer token")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("-f is required")
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	var req map[string]any
	if err := yaml.Unmarshal(raw, &req); err != nil {
		return fmt.Errorf("parse %s: %w", *file, err)
	}
	if name, _ := req["name"].(string); strings.TrimSpace(name) == "" {
		return fmt.Errorf("name is required in %s", *file)
	}

	client, err := corecli.FromContext(*server, *token)
	if err != nil {
		return err
	}
	var provider map[string]any
	if err := client.Do(context.Background(), http.MethodPost, "/api/v1/auth/providers", req, &provider); err != nil {
		return err
	}
	name, _ := provider["name"].(string)
	fmt.Printf("provider %q applied\n", name)

	// Surface the post-apply login URL so the operator can verify
	// immediately. The path matches what handleAuthThirdPartyStart
	// registers on /api/v1/auth/third-party/start/{provider}.
	if active := mustActiveServer(); active != "" {
		fmt.Printf("login URL: %s/api/v1/auth/third-party/start/%s\n", active, name)
	}
	_ = json.RawMessage{}
	return nil
}

func runAuthProviderDelete(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: yggdrasil auth provider delete <name>")
	}
	name := args[0]
	client, err := corecli.FromContext("", "")
	if err != nil {
		return err
	}
	if err := client.Do(context.Background(), http.MethodDelete, "/api/v1/auth/providers/"+name, nil, nil); err != nil {
		return err
	}
	fmt.Printf("provider %q deleted\n", name)
	return nil
}

func mustActiveServer() string {
	cfg, _, err := corecli.Load()
	if err != nil || cfg == nil {
		return ""
	}
	if active, _ := cfg.Active(); active != nil {
		return active.Server
	}
	return ""
}
