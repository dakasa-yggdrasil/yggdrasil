package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// repeatedString collects a flag that may be passed more than once
// (e.g. --input k=v --input k2=v2).
type repeatedString []string

func (r *repeatedString) String() string  { return strings.Join(*r, ",") }
func (r *repeatedString) Set(v string) error { *r = append(*r, v); return nil }

// parseInputPairs turns ["k=v", ...] into a map. Values are kept as
// strings — workflow input schemas coerce types server-side, and a CLI
// that guessed int/bool/json would surprise more than it helped.
func parseInputPairs(pairs []string) (map[string]any, error) {
	out := make(map[string]any, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid --input %q (want key=value)", p)
		}
		out[k] = v
	}
	return out, nil
}

// runRun implements `yggdrasil run <workflow> [flags]` — dispatch a
// workflow run by name. Sync by default (prints per-step progress);
// --async returns a run id immediately for `yggdrasil logs`/`ops get`.
func runRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	namespace := fs.String("namespace", "global", "workflow namespace")
	fs.StringVar(namespace, "n", "global", "short for --namespace")
	var inputs repeatedString
	fs.Var(&inputs, "input", "workflow input as key=value (repeatable)")
	async := fs.Bool("async", false, "dispatch asynchronously and return a run id immediately")
	server := fs.String("server", "", "override the active context's server URL")
	token := fs.String("token", "", "override the active context's bearer token")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `yggdrasil run — dispatch a workflow run by name

Usage:
  yggdrasil run <workflow> [-n <ns>] [--input k=v ...]
  yggdrasil run <workflow> --async        # return a run id, don't block

Flags:
`)
		fs.PrintDefaults()
	}

	name, _, flagArgs := splitGetPositionals(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if name == "" {
		fs.Usage()
		return errors.New("workflow name is required")
	}

	inputMap, err := parseInputPairs(inputs)
	if err != nil {
		return err
	}

	client, err := clientFromFlags(*server, *token)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	res, err := client.RunWorkflowMode(ctx, *namespace, name, inputMap, *async)
	if err != nil {
		return fmt.Errorf("run workflow: %w", err)
	}

	if *async {
		fmt.Printf("✓ dispatched %s/%s (run %s, status %s)\n", *namespace, name, res.RunID, res.Status)
		if res.RunID != "" {
			fmt.Printf("  follow: yggdrasil logs %s   |   yggdrasil ops get %s\n", res.RunID, res.RunID)
		}
		return nil
	}

	printWorkflowRunResult(res)
	if !isTerminalSuccess(res.Status) {
		return fmt.Errorf("workflow run status: %s", res.Status)
	}
	return nil
}

// isTerminalSuccess treats both succeeded and committed as success — the
// core promoted `committed` to a success terminal (yggdrasil-core commit
// 0539356), and deploy.go's success check predates that.
func isTerminalSuccess(status string) bool {
	switch strings.ToLower(status) {
	case "succeeded", "committed":
		return true
	default:
		return false
	}
}
