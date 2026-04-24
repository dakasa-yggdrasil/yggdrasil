package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/scaffoldcli"
)

// runNew dispatches `yggdrasil new integration|surface ...`. The first
// positional argument picks the kind; everything else goes to
// scaffoldcli.Options.
func runNew(args []string) error {
	if len(args) < 2 {
		fmt.Println(`yggdrasil new — scaffold a new Yggdrasil plugin from an official template

Usage:
  yggdrasil new integration <name> [flags]      scaffold a new integration adapter
  yggdrasil new surface <name> [flags]          scaffold a new UI / edge surface

Flags:
  --module <path>       Go module path (default: github.com/<owner>/<kind>-<name>)
  --owner <org>         GitHub owner for default module + install hint
  --dir <path>          target directory (default: ./<kind>-<name>)
  --template <ref>      override template repo (default: dakasa-yggdrasil/<kind>-template)
  --no-git-init         skip git init in the scaffolded directory

Example:
  yggdrasil new integration datadog --owner acme`)
		return fmt.Errorf("kind and name are required")
	}
	kind := scaffoldcli.Kind(args[0])
	name := args[1]

	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	module := fs.String("module", "", "Go module path for the new plugin")
	owner := fs.String("owner", "", "GitHub owner (for default module + install hint)")
	dir := fs.String("dir", "", "target directory")
	template := fs.String("template", "", "override template repo (owner/repo)")
	skipGit := fs.Bool("no-git-init", false, "skip git init in the new directory")

	if err := fs.Parse(args[2:]); err != nil {
		return err
	}

	ctx := context.Background()
	result, err := scaffoldcli.Run(ctx, scaffoldcli.Options{
		Kind:         kind,
		Name:         name,
		Module:       *module,
		GitHubOwner:  *owner,
		Dir:          *dir,
		TemplateRepo: *template,
		SkipGitInit:  *skipGit,
	})
	if err != nil {
		return err
	}

	printScaffoldBanner(result)
	return nil
}

func printScaffoldBanner(r scaffoldcli.Result) {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#6fb1fc")).Render
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#888")).Render
	value := lipgloss.NewStyle().Bold(true).Render

	fmt.Println()
	fmt.Println(title("✓ scaffold ready"))
	fmt.Println()
	fmt.Printf("  %s %s\n", label("directory:"), value(r.Dir))
	fmt.Printf("  %s %s\n", label("module:   "), value(r.Module))
	fmt.Printf("  %s %s\n", label("project:  "), value(r.ProjectName))
	fmt.Println()
	fmt.Println(label("Next steps:"))
	fmt.Printf("  cd %s\n", r.ProjectName)
	fmt.Println("  go test ./...                                    # the template compiles out of the box")
	fmt.Println("  # edit internal/adapter/spec.go and add your operations")
	fmt.Println("  git commit -am \"initial commit\"")
	fmt.Println("  gh repo create --public --push")
	fmt.Println()
	fmt.Println(label("When your plugin is on GitHub, any adopter can install it with:"))
	fmt.Printf("  %s\n", value(r.InstallHint))
	fmt.Println()
	os.Stderr.Sync()
}
