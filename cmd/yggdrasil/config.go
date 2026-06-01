package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

// runConfig implements `yggdrasil config <subcommand>` — kubectl-style
// management of ~/.yggdrasil/config.yaml so adopters can switch servers
// without hand-editing YAML.
func runConfig(args []string) error {
	if len(args) == 0 {
		printConfigUsage()
		return errors.New("config requires a subcommand")
	}
	switch args[0] {
	case "get-contexts":
		return runConfigGetContexts()
	case "current-context":
		return runConfigCurrentContext()
	case "use-context":
		return runConfigUseContext(args[1:])
	case "set-context":
		return runConfigSetContext(args[1:])
	case "delete-context":
		return runConfigDeleteContext(args[1:])
	case "view":
		return runConfigView()
	case "help", "-h", "--help":
		printConfigUsage()
		return nil
	default:
		return fmt.Errorf("unknown config subcommand %q (want get-contexts|current-context|use-context|set-context|delete-context|view)", args[0])
	}
}

func printConfigUsage() {
	fmt.Fprint(os.Stderr, `yggdrasil config — manage ~/.yggdrasil/config.yaml contexts

Usage:
  yggdrasil config get-contexts
  yggdrasil config current-context
  yggdrasil config use-context <name>
  yggdrasil config set-context <name> [--server url] [--token t] [--collaborator slug]
  yggdrasil config delete-context <name>
  yggdrasil config view
`)
}

func runConfigGetContexts() error {
	cfg, _, err := corecli.Load()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(cfg.Contexts))
	for name := range cfg.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		marker := "  "
		if name == cfg.CurrentContext {
			marker = "* "
		}
		fmt.Printf("%s%s\t%s\n", marker, name, cfg.Contexts[name].Server)
	}
	return nil
}

func runConfigCurrentContext() error {
	cfg, _, err := corecli.Load()
	if err != nil {
		return err
	}
	if cfg.CurrentContext == "" {
		return errors.New("no current context set")
	}
	fmt.Println(cfg.CurrentContext)
	return nil
}

func runConfigUseContext(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return errors.New("usage: yggdrasil config use-context <name>")
	}
	name := args[0]
	cfg, _, err := corecli.Load()
	if err != nil {
		return err
	}
	if _, ok := cfg.Contexts[name]; !ok {
		return fmt.Errorf("no context named %q (see `yggdrasil config get-contexts`)", name)
	}
	cfg.CurrentContext = name
	if err := corecli.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("switched to context %q\n", name)
	return nil
}

func runConfigSetContext(args []string) error {
	fs := flag.NewFlagSet("config set-context", flag.ContinueOnError)
	server := fs.String("server", "", "server URL for this context")
	token := fs.String("token", "", "bearer token for this context")
	collaborator := fs.String("collaborator", "", "collaborator slug for this context")
	name, _, flagArgs := splitGetPositionals(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if name == "" {
		return errors.New("usage: yggdrasil config set-context <name> [--server url] [--token t]")
	}
	cfg, _, err := corecli.Load()
	if err != nil {
		return err
	}
	// Merge onto any existing context so a partial update (e.g. only
	// --token) preserves the rest.
	ctx := cfg.Contexts[name]
	if ctx == nil {
		ctx = &corecli.Context{}
	}
	if *server != "" {
		ctx.Server = *server
	}
	if *token != "" {
		ctx.Token = *token
	}
	if *collaborator != "" {
		ctx.Collaborator = *collaborator
	}
	cfg.SetContext(name, ctx, false)
	if err := corecli.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("set context %q (server=%s)\n", name, ctx.Server)
	return nil
}

func runConfigDeleteContext(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return errors.New("usage: yggdrasil config delete-context <name>")
	}
	name := args[0]
	cfg, _, err := corecli.Load()
	if err != nil {
		return err
	}
	if _, ok := cfg.Contexts[name]; !ok {
		return fmt.Errorf("no context named %q", name)
	}
	delete(cfg.Contexts, name)
	if cfg.CurrentContext == name {
		cfg.CurrentContext = ""
	}
	if err := corecli.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("deleted context %q\n", name)
	return nil
}

func runConfigView() error {
	cfg, path, err := corecli.Load()
	if err != nil {
		return err
	}
	// Redact tokens — `view` is for sharing/debugging, not secret export.
	for _, ctx := range cfg.Contexts {
		if ctx.Token != "" {
			ctx.Token = "REDACTED"
		}
	}
	fmt.Printf("# %s\n", path)
	fmt.Printf("current-context: %s\n", cfg.CurrentContext)
	names := make([]string, 0, len(cfg.Contexts))
	for name := range cfg.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		c := cfg.Contexts[name]
		fmt.Printf("- %s: server=%s collaborator=%s token=%s\n", name, c.Server, c.Collaborator, c.Token)
	}
	return nil
}
