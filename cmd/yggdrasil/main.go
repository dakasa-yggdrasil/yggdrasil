package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/integrations"
	"github.com/dakasa-yggdrasil/yggdrasil/internal/surfaces"
)

func main() {
	root, err := findRoot()
	if err != nil {
		exitErr(err)
	}

	manager, err := integrations.NewManager(root)
	if err != nil {
		exitErr(err)
	}
	surfaceManager := surfaces.NewManager(root)

	if len(os.Args) < 2 {
		printUsage()
		return
	}

	switch os.Args[1] {
	case "integrations":
		if err := runIntegrations(manager, os.Args[2:]); err != nil {
			exitErr(err)
		}
	case "surfaces":
		if err := runSurfaces(surfaceManager, os.Args[2:]); err != nil {
			exitErr(err)
		}
	case "install":
		if err := runInstall(os.Args[2:]); err != nil {
			exitErr(err)
		}
	case "help", "-h", "--help":
		printUsage()
	default:
		exitErr(fmt.Errorf("unknown command: %s", os.Args[1]))
	}
}

func runSurfaces(manager *surfaces.Manager, args []string) error {
	if len(args) == 0 {
		printSurfacesUsage()
		return nil
	}

	switch args[0] {
	case "list":
		items, err := manager.List()
		if err != nil {
			return err
		}
		fmt.Print(surfaces.RenderTable(items))
		return nil
	case "active":
		items, err := manager.Active()
		if err != nil {
			return err
		}
		for _, item := range items {
			fmt.Println(item)
		}
		return nil
	case "install":
		if len(args) < 2 {
			return errors.New("usage: yggdrasil surfaces install <name>")
		}
		entry, source, err := manager.Install(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("installed %s via %s in surfaces/%s\n", entry.Slug, source, entry.RepoName)
		return nil
	case "remove":
		if len(args) < 2 {
			return errors.New("usage: yggdrasil surfaces remove <name>")
		}
		entry, err := manager.Remove(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("removed %s from surfaces/%s\n", entry.Slug, entry.RepoName)
		return nil
	case "installed":
		files, err := manager.ComposeFiles()
		if err != nil {
			return err
		}
		for _, file := range files {
			fmt.Println(file)
		}
		return nil
	case "scaffold":
		if len(args) < 2 {
			return errors.New("usage: yggdrasil surfaces scaffold <name> [module]")
		}
		module := ""
		if len(args) > 2 {
			module = args[2]
		}
		slug, path, err := manager.Scaffold(args[1], module)
		if err != nil {
			return err
		}
		fmt.Printf("scaffolded surface %s at %s\n", slug, path)
		return nil
	case "activate":
		if len(args) < 2 {
			return errors.New("usage: yggdrasil surfaces activate <name>")
		}
		if err := manager.Activate(args[1]); err != nil {
			return err
		}
		fmt.Printf("activated surface %s\n", args[1])
		return nil
	case "deactivate":
		if len(args) < 2 {
			return errors.New("usage: yggdrasil surfaces deactivate <name>")
		}
		if err := manager.Deactivate(args[1]); err != nil {
			return err
		}
		fmt.Printf("deactivated surface %s\n", args[1])
		return nil
	case "help", "-h", "--help":
		printSurfacesUsage()
		return nil
	default:
		return fmt.Errorf("unknown surfaces command: %s", args[0])
	}
}

func runIntegrations(manager *integrations.Manager, args []string) error {
	if len(args) == 0 {
		printIntegrationsUsage()
		return nil
	}

	switch args[0] {
	case "list":
		entries, err := manager.List()
		if err != nil {
			return err
		}
		fmt.Print(integrations.RenderTable(entries))
		return nil
	case "install":
		if len(args) < 2 {
			return errors.New("usage: yggdrasil integrations install <slug>")
		}
		entry, source, err := manager.Install(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("installed %s from %s into integrations/%s\n", entry.Slug, source, entry.RepoName)
		return nil
	case "remove":
		if len(args) < 2 {
			return errors.New("usage: yggdrasil integrations remove <slug>")
		}
		entry, err := manager.Remove(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("removed %s from integrations/%s\n", entry.Slug, entry.RepoName)
		return nil
	case "tui":
		model, err := integrations.NewTUIModel(manager)
		if err != nil {
			return err
		}
		_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
		return err
	case "installed":
		files, err := manager.ComposeFiles()
		if err != nil {
			return err
		}
		for _, file := range files {
			fmt.Println(file)
		}
		return nil
	case "help", "-h", "--help":
		printIntegrationsUsage()
		return nil
	default:
		return fmt.Errorf("unknown integrations command: %s", args[0])
	}
}

func findRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		catalogPath := filepath.Join(wd, "catalog", "integrations.json")
		if _, err := os.Stat(catalogPath); err == nil {
			return wd, nil
		}

		parent := filepath.Dir(wd)
		if parent == wd {
			return "", errors.New("could not find yggdrasil workspace root")
		}
		wd = parent
	}
}

func printUsage() {
	fmt.Println(`yggdrasil - Yggdrasil platform CLI

Usage:
  yggdrasil install <repo_ref>                       quickstart-install an integration on a remote yggdrasil-core
  yggdrasil integrations list
  yggdrasil integrations install <slug>              clone an integration into the local workspace (dev mode)
  yggdrasil integrations remove <slug>
  yggdrasil integrations tui
  yggdrasil integrations installed
  yggdrasil surfaces list
  yggdrasil surfaces active
  yggdrasil surfaces install <name>
  yggdrasil surfaces remove <name>
  yggdrasil surfaces installed
  yggdrasil surfaces scaffold <name> [module]
  yggdrasil surfaces activate <name>
  yggdrasil surfaces deactivate <name>

Run 'yggdrasil install --help' for the quickstart flow's flags.`)
}

func printIntegrationsUsage() {
	fmt.Println(`yggdrasil integrations - workspace dev helper

Usage:
  yggdrasil integrations list
  yggdrasil integrations install <slug>
  yggdrasil integrations remove <slug>
  yggdrasil integrations tui
  yggdrasil integrations installed

To install an integration on a REMOTE yggdrasil-core (the adopter flow), use:
  yggdrasil install <repo_ref>`)
}

func printSurfacesUsage() {
	fmt.Println(`yggdrasil surfaces

Usage:
  yggdrasil surfaces list
  yggdrasil surfaces active
  yggdrasil surfaces install <name>
  yggdrasil surfaces remove <name>
  yggdrasil surfaces installed
  yggdrasil surfaces scaffold <name> [module]
  yggdrasil surfaces activate <name>
  yggdrasil surfaces deactivate <name>`)
}

func exitErr(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
