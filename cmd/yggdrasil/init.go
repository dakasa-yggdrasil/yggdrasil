package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/initcli"
)

// runInit implements `yggdrasil init`. See internal/initcli for the flow.
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	dir := fs.String("dir", "./yggdrasil", "target directory for the compose stack")
	server := fs.String("server", "", "connect to an existing yggdrasil-core (skip docker compose)")
	adminUsername := fs.String("admin-username", "admin", "first admin slug (becomes login identifier)")
	adminPassword := fs.String("admin-password", "", "first admin password (empty = random, printed at the end)")
	adminEmail := fs.String("admin-email", "", "first admin email (optional)")
	adminDisplayName := fs.String("admin-display-name", "Admin", "first admin display name")
	coreImage := fs.String("core-image", initcli.DefaultCoreImage, "yggdrasil-core container image")
	httpPort := fs.Int("port", 9080, "host port for the core HTTP listener")
	contextName := fs.String("context", "", "name to store under in ~/.yggdrasil/config.yaml")
	yes := fs.Bool("yes", false, "skip confirmation prompts")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `yggdrasil init — bootstrap a self-hosted yggdrasil-core in one command

Usage:
  yggdrasil init [--dir <path>]            standalone docker-compose stack
  yggdrasil init --server <url>            attach to an existing yggdrasil-core

Flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()
	result, err := initcli.Run(ctx, initcli.Options{
		Dir:              *dir,
		Server:           *server,
		AdminUsername:    *adminUsername,
		AdminPassword:    *adminPassword,
		AdminEmail:       *adminEmail,
		AdminDisplayName: *adminDisplayName,
		CoreImage:        *coreImage,
		HTTPPort:         *httpPort,
		ContextName:      *contextName,
		Yes:              *yes,
	})
	if err != nil {
		return err
	}

	printInitBanner(result)
	return nil
}

func printInitBanner(r initcli.Result) {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#6fb1fc")).Render
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#888")).Render
	value := lipgloss.NewStyle().Bold(true).Render
	warn := lipgloss.NewStyle().Foreground(lipgloss.Color("#f5a742")).Render

	fmt.Println()
	fmt.Println(title("✓ yggdrasil is ready"))
	fmt.Println()
	if r.Dir != "" {
		fmt.Printf("  %s %s\n", label("directory:"), value(r.Dir))
	}
	fmt.Printf("  %s %s\n", label("server:   "), value(r.Server))
	fmt.Printf("  %s %s\n", label("context:  "), value(r.ContextName))
	if r.ConfigPath != "" {
		fmt.Printf("  %s %s\n", label("config:   "), value(r.ConfigPath))
	}
	fmt.Println()
	fmt.Printf("  %s %s\n", label("username: "), value(r.AdminUsername))
	if r.AdminPasswordGenerated {
		fmt.Printf("  %s %s  %s\n", label("password: "), value(r.AdminPassword), warn("(generated — save it now)"))
	}
	fmt.Println()
	fmt.Println(label("Next steps:"))
	fmt.Println("  yggdrasil status                                 confirm everything is up")
	fmt.Println("  yggdrasil install dakasa-yggdrasil/integration-kubernetes --provider kubernetes")
	fmt.Println("  yggdrasil apply -f my-workflow.yaml")
	fmt.Println()
}
