package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

// runLogs implements `yggdrasil logs <workflow-run-id>`. Polls the
// run endpoint and prints step status transitions until the run
// reaches a terminal state. Streaming-style output gives an adopter
// the same feel as `kubectl logs -f` for one-off workflow runs.
//
// The endpoint shape mirrors what the install/dispatch flow already
// returns: GET /api/v1/workflow_runs/<id> returns a structure with
// status + steps[] array.
func runLogs(args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	follow := fs.Bool("f", true, "follow the run until it terminates (succeeded/failed)")
	intervalSec := fs.Int("interval", 2, "polling interval in seconds (only meaningful with -f)")
	timeoutSec := fs.Int("timeout", 600, "give up after N seconds")
	server := fs.String("server", "", "override context server URL")
	token := fs.String("token", "", "override context bearer token")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `yggdrasil logs — stream a workflow run's step results

Usage:
  yggdrasil logs <run-id>

Flags:
`)
		fs.PrintDefaults()
	}

	var positional []string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			rest = append(rest, a)
			if i+1 < len(args) && !strings.Contains(a, "=") && !strings.HasPrefix(args[i+1], "-") {
				rest = append(rest, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, a)
	}
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if len(positional) < 1 {
		fs.Usage()
		return fmt.Errorf("run id is required")
	}
	runID := positional[0]

	client, err := corecli.FromContext(*server, *token)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(time.Duration(*timeoutSec) * time.Second)
	seenSteps := map[string]string{} // stepID -> last status

	for {
		var run map[string]any
		err := client.Do(context.Background(), http.MethodGet, "/api/v1/workflow_runs/"+runID, nil, &run)
		if err != nil {
			return err
		}

		status, _ := run["status"].(string)
		steps, _ := run["steps"].([]any)

		for _, raw := range steps {
			step, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			stepID, _ := step["id"].(string)
			stepStatus, _ := step["status"].(string)
			if seenSteps[stepID] != stepStatus {
				printStepLine(step)
				seenSteps[stepID] = stepStatus
			}
		}

		if !*follow || isTerminalStatus(status) {
			fmt.Printf("\nrun %s: %s\n", runID, status)
			if status == "failed" {
				return fmt.Errorf("workflow run %s failed", runID)
			}
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %ds; run %s is still %s", *timeoutSec, runID, status)
		}

		time.Sleep(time.Duration(*intervalSec) * time.Second)
	}
}

func printStepLine(step map[string]any) {
	id, _ := step["id"].(string)
	status, _ := step["status"].(string)
	op, _ := step["operation"].(string)
	errMsg, _ := step["error"].(string)
	prefix := "→"
	switch status {
	case "succeeded":
		prefix = "✓"
	case "failed":
		prefix = "✗"
	case "skipped":
		prefix = "·"
	}
	if errMsg != "" {
		fmt.Printf("%s %s [%s] %s — %s\n", prefix, id, status, op, errMsg)
		return
	}
	fmt.Printf("%s %s [%s] %s\n", prefix, id, status, op)
}

func isTerminalStatus(s string) bool {
	switch s {
	case "succeeded", "failed", "cancelled":
		return true
	}
	return false
}
