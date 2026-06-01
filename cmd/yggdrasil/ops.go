package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

// runOps implements `yggdrasil ops <subcommand>` — the operate-side view
// of workflow runs that the core exposes under /api/v1/ops/workflows.
func runOps(args []string) error {
	if len(args) == 0 {
		printOpsUsage()
		return errors.New("ops requires a subcommand (list|get|retry|abort|replay)")
	}
	switch args[0] {
	case "list":
		return runOpsList(args[1:])
	case "get":
		return runOpsGet(args[1:])
	case "retry", "abort", "replay":
		return runOpsAction(args[0], args[1:])
	case "help", "-h", "--help":
		printOpsUsage()
		return nil
	default:
		return fmt.Errorf("unknown ops subcommand %q (want list|get|retry|abort|replay)", args[0])
	}
}

func printOpsUsage() {
	fmt.Fprint(os.Stderr, `yggdrasil ops — inspect and manage workflow runs

Usage:
  yggdrasil ops list [--status s ...] [--integration i] [--limit N] [-o table|json]
  yggdrasil ops get <run-id> [-o yaml|json]
  yggdrasil ops retry <run-id> [--reason r]
  yggdrasil ops abort <run-id>
  yggdrasil ops replay <run-id>
`)
}

func runOpsList(args []string) error {
	fs := flag.NewFlagSet("ops list", flag.ContinueOnError)
	var status repeatedString
	fs.Var(&status, "status", "filter by status (repeatable)")
	integration := fs.String("integration", "", "filter by integration")
	search := fs.String("search", "", "substring search over run/workflow")
	limit := fs.Int("limit", 50, "maximum rows to return")
	output := fs.String("o", "table", "output format: table | json")
	server := fs.String("server", "", "override the active context's server URL")
	token := fs.String("token", "", "override the active context's bearer token")
	if err := fs.Parse(args); err != nil {
		return err
	}
	client, err := clientFromFlags(*server, *token)
	if err != nil {
		return err
	}
	page, err := client.ListRuns(context.Background(), corecli.OpsListOptions{
		Status:      status,
		Integration: *integration,
		Search:      *search,
		Limit:       *limit,
	})
	if err != nil {
		return err
	}
	if *output == "json" {
		return printJSON(page.Runs)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RUN_ID\tSTATUS\tWORKFLOW\tINTEGRATION\tTRIGGER")
	for _, r := range page.Runs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.RunID, r.Status, r.WorkflowName, r.Integration, r.TriggerSource)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if page.NextCursor != "" {
		fmt.Printf("\n(more results — re-run with --limit or a cursor; next_cursor=%s)\n", page.NextCursor)
	}
	return nil
}

func runOpsGet(args []string) error {
	fs := flag.NewFlagSet("ops get", flag.ContinueOnError)
	output := fs.String("o", "yaml", "output format: yaml | json")
	server := fs.String("server", "", "override the active context's server URL")
	token := fs.String("token", "", "override the active context's bearer token")
	runID, _, flagArgs := splitGetPositionals(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if runID == "" {
		return errors.New("usage: yggdrasil ops get <run-id>")
	}
	client, err := clientFromFlags(*server, *token)
	if err != nil {
		return err
	}
	detail, err := client.GetRun(context.Background(), runID)
	if err != nil {
		return err
	}
	if *output == "json" {
		return printJSON(detail)
	}
	out, err := yaml.Marshal(detail)
	if err != nil {
		return err
	}
	fmt.Print(string(out))
	return nil
}

func runOpsAction(verb string, args []string) error {
	fs := flag.NewFlagSet("ops "+verb, flag.ContinueOnError)
	reason := fs.String("reason", "", "optional reason recorded in the audit trail (retry)")
	server := fs.String("server", "", "override the active context's server URL")
	token := fs.String("token", "", "override the active context's bearer token")
	runID, _, flagArgs := splitGetPositionals(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if runID == "" {
		return fmt.Errorf("usage: yggdrasil ops %s <run-id>", verb)
	}
	client, err := clientFromFlags(*server, *token)
	if err != nil {
		return err
	}
	resp, err := client.RunOp(context.Background(), runID, verb, *reason)
	if err != nil {
		return err
	}
	if newID, ok := resp["new_run_id"].(string); ok && newID != "" {
		fmt.Printf("✓ %s %s → new run %s\n", verb, runID, newID)
		return nil
	}
	fmt.Printf("✓ %s %s accepted\n", verb, runID)
	return nil
}

// printJSON renders any value as indented JSON to stdout.
func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
