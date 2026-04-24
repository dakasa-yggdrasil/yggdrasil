package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/quickstartcli"
)

// runInstall implements `yggdrasil install <repo_ref> [flags]`. It fetches
// the repo's yggdrasil-quickstart.yaml, walks the user through the
// declared inputs (interactive TUI by default, or headless when
// --non-interactive is set), and POSTs the install request to a remote
// yggdrasil-core's /api/v1/integrations/install endpoint.
func runInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	server := fs.String("server", os.Getenv("YGGDRASIL_URL"), "yggdrasil-core base URL (defaults to $YGGDRASIL_URL)")
	token := fs.String("token", os.Getenv("YGGDRASIL_WORKFLOW_RUN_TOKEN"), "bearer token for the install endpoint (defaults to $YGGDRASIL_WORKFLOW_RUN_TOKEN)")
	provider := fs.String("provider", "", "provider id to install (skips the picker when set)")
	dryRun := fs.Bool("dry-run", false, "ask the server to compile the workflow without dispatching it")
	nonInteractive := fs.Bool("non-interactive", false, "skip the TUI and read all inputs from --input flags / defaults (CI mode)")
	yes := fs.Bool("yes", false, "skip the final confirmation prompt (implied by --non-interactive)")
	var seedFlags multiStringFlag
	fs.Var(&seedFlags, "input", "pre-fill an input as key=value; may be repeated")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `yggdrasil install — quickstart-install an integration on a remote yggdrasil-core

Usage:
  yggdrasil install <repo_ref> [--provider id] [--input k=v ...] [--dry-run] [--non-interactive]

repo_ref:
  owner/repo                          GitHub: default branch + yggdrasil-quickstart.yaml
  owner/repo@v1.2.3                   GitHub: pinned ref
  owner/repo:custom/path.yaml         GitHub: custom manifest path
  owner/repo@ref:path                 GitHub: both
  oci://ghcr.io/owner/repo            OCI: latest tag
  oci://ghcr.io/owner/repo:v1.2.3     OCI: specific tag

Flags:
`)
		fs.PrintDefaults()
	}

	// flag.Parse stops at the first non-flag arg, which makes "yggdrasil
	// install <repo_ref> --flags ..." silently drop the trailing flags.
	// Reorder: pull the first non-flag arg out as repo_ref and parse the
	// rest as flags. Keeps the natural CLI ordering working.
	repoRefRaw, flagArgs := splitRepoRefAndFlags(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if repoRefRaw == "" {
		fs.Usage()
		return fmt.Errorf("repo_ref is required")
	}

	ref, err := quickstartcli.ParseRepoRef(repoRefRaw)
	if err != nil {
		return err
	}

	seeds, err := parseSeedFlags(seedFlags)
	if err != nil {
		return err
	}

	ctx := context.Background()
	doc, raw, err := quickstartcli.FetchManifest(ctx, ref)
	if err != nil {
		return err
	}

	if !*nonInteractive {
		quickstartcli.PrintBanner(doc, ref)
	}

	chosenProvider, err := quickstartcli.PickProvider(doc.Spec, *provider)
	if err != nil {
		return err
	}

	if !*nonInteractive {
		quickstartcli.PrintRequirements(chosenProvider)
	}

	var inputs map[string]any
	if *nonInteractive {
		inputs, err = quickstartcli.CollectInputsHeadless(chosenProvider, seeds)
	} else {
		inputs, err = quickstartcli.CollectInputs(chosenProvider, seeds)
	}
	if err != nil {
		return err
	}

	if !*nonInteractive && !*yes {
		confirmed, err := quickstartcli.ConfirmInstall(chosenProvider, inputs, *dryRun)
		if err != nil {
			return err
		}
		if !confirmed {
			return fmt.Errorf("aborted by user")
		}
	}

	// For OCI refs, pass the already-fetched bytes inline so the
	// server doesn't need to re-authenticate to the registry. For
	// GitHub refs the server can fetch via raw.githubusercontent
	// without extra auth — cleaner audit trail when the server does
	// the fetch itself.
	var inlineManifest []byte
	if ref.OCI != nil {
		inlineManifest = raw
	}

	client := quickstartcli.NewClient(*server, *token)
	resp, err := client.Install(ctx, quickstartcli.InstallRequest{
		RepoRef:        ref.String(),
		ManifestInline: inlineManifest,
		ProviderID:     chosenProvider.ID,
		Inputs:         inputs,
		DryRun:         *dryRun,
	})
	if err != nil {
		return err
	}

	quickstartcli.PrintResult(resp, *dryRun)
	return nil
}

// parseSeedFlags turns ["region=us-east-1","tier=standard"] into a map.
func parseSeedFlags(flags multiStringFlag) (map[string]string, error) {
	out := make(map[string]string, len(flags))
	for _, raw := range flags {
		i := strings.Index(raw, "=")
		if i <= 0 {
			return nil, fmt.Errorf("--input %q must be key=value", raw)
		}
		out[strings.TrimSpace(raw[:i])] = raw[i+1:]
	}
	return out, nil
}

// multiStringFlag accumulates a flag that can appear more than once
// (e.g. --input region=us-east-1 --input tier=standard).
type multiStringFlag []string

func (m *multiStringFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiStringFlag) Set(s string) error { *m = append(*m, s); return nil }

// installBoolFlags lists the flag names this subcommand declares as
// booleans. Used by splitRepoRefAndFlags to decide whether the token after
// "--foo" is its value (string flag) or the next positional (bool flag).
var installBoolFlags = map[string]bool{
	"dry-run":         true,
	"non-interactive": true,
	"yes":             true,
	"h":               true,
	"help":            true,
}

// splitRepoRefAndFlags pulls the first non-flag argument out of args (the
// repo_ref) and returns it alongside the rest. This lets the CLI accept
// "install <ref> --flags" — the stdlib flag package would otherwise stop
// at <ref> and silently drop the flags after it.
//
// Boolean flags (see installBoolFlags) DON'T consume the next arg, so a
// token sitting between "--non-interactive" and another flag is the
// repo_ref. Other flags ("--server URL", "--input k=v") DO consume it.
func splitRepoRefAndFlags(args []string) (string, []string) {
	flags := make([]string, 0, len(args))
	var repoRef string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			if repoRef == "" {
				repoRef = a
				continue
			}
			flags = append(flags, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if eq := strings.Index(name, "="); eq >= 0 {
			name = name[:eq]
			continue // value already attached
		}
		if installBoolFlags[name] {
			continue // bool flag — does not consume next arg
		}
		// String flag with space-separated value. Consume it.
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return repoRef, flags
}
