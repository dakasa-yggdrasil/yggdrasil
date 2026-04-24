package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

// runDeploy implements `yggdrasil deploy <target>`. Today one target:
//
//	yggdrasil deploy control-plane -f control-plane.yaml
//
// The command applies a `control_plane` manifest and then dispatches
// the seeded workflow `yggdrasil-deploy-control-plane` against it,
// printing per-step progress as a single synchronous command. It is
// sugar over `apply` + `workflow run` that encodes the sensible
// defaults (kubernetes_instance, schema_migrations_instance) the
// bootstrap topology ships with.
func runDeploy(args []string) error {
	if len(args) == 0 {
		return errors.New("deploy requires a target (e.g. `yggdrasil deploy control-plane -f ...`)")
	}
	switch strings.ToLower(args[0]) {
	case "control-plane", "controlplane", "control_plane":
		return runDeployControlPlane(args[1:])
	default:
		return fmt.Errorf("unknown deploy target %q (supported: control-plane)", args[0])
	}
}

func runDeployControlPlane(args []string) error {
	fs := flag.NewFlagSet("deploy control-plane", flag.ContinueOnError)
	file := fs.String("f", "", "path to a control_plane manifest file (YAML or JSON; use '-' for stdin)")
	server := fs.String("server", "", "override the active context's server URL")
	token := fs.String("token", "", "override the active context's bearer token")
	noRun := fs.Bool("no-run", false, "apply the manifest but skip the workflow run")
	kubernetesInstance := fs.String("kubernetes-instance", "yggdrasil-core-kubernetes", "integration_instance name used by the deploy workflow")
	schemaMigrationsInstance := fs.String("schema-migrations-instance", "yggdrasil-core-schema-migrations", "integration_instance name used by the deploy workflow")
	instanceNamespace := fs.String("instance-namespace", "global", "namespace where the integration_instance objects live")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `yggdrasil deploy control-plane — apply a control_plane manifest and run the deploy workflow

Usage:
  yggdrasil deploy control-plane -f control-plane.yaml
  yggdrasil deploy control-plane -f - < control-plane.yaml
  yggdrasil deploy control-plane -f cp.yaml --no-run

Flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*file) == "" {
		fs.Usage()
		return errors.New("-f is required")
	}

	doc, err := loadControlPlaneDocument(*file)
	if err != nil {
		return err
	}
	metadata, _ := doc["metadata"].(map[string]any)
	manifestName, _ := metadata["name"].(string)
	manifestNamespace, _ := metadata["namespace"].(string)
	if manifestNamespace == "" {
		manifestNamespace = "global"
	}
	if manifestName == "" {
		return errors.New("control_plane manifest has no metadata.name")
	}

	client, err := clientFromFlags(*server, *token)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if _, err := client.CreateManifest(ctx, "control_plane", doc); err != nil {
		return fmt.Errorf("apply control_plane manifest: %w", err)
	}
	fmt.Printf("✓ applied control_plane %s/%s\n", manifestNamespace, manifestName)

	if *noRun {
		fmt.Println("skipping workflow run (--no-run)")
		return nil
	}

	inputs := map[string]any{
		"control_plane": map[string]any{
			"namespace": manifestNamespace,
			"name":      manifestName,
		},
		"kubernetes_instance": map[string]any{
			"namespace": *instanceNamespace,
			"name":      *kubernetesInstance,
		},
		"schema_migrations_instance": map[string]any{
			"namespace": *instanceNamespace,
			"name":      *schemaMigrationsInstance,
		},
	}

	fmt.Printf("→ dispatching workflow yggdrasil-deploy-control-plane\n")
	result, err := client.RunWorkflow(ctx, "global", "yggdrasil-deploy-control-plane", inputs)
	if err != nil {
		return fmt.Errorf("run workflow: %w", err)
	}

	printWorkflowRunResult(result)

	if !strings.EqualFold(result.Status, "succeeded") {
		return fmt.Errorf("workflow run status: %s", result.Status)
	}
	return nil
}

// loadControlPlaneDocument reads a YAML or JSON file into a map. If
// the document declares a non-control_plane kind we fail loudly —
// yggdrasil apply is the right tool for any other manifest shape.
func loadControlPlaneDocument(path string) (map[string]any, error) {
	var data []byte
	if path == "-" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		data = buf
	} else {
		buf, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		data = buf
	}
	jsonBytes, err := yaml.YAMLToJSON(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	kind, _ := doc["kind"].(string)
	if !strings.EqualFold(kind, "control_plane") {
		return nil, fmt.Errorf("expected kind=control_plane, got %q (use `yggdrasil apply` for other kinds)", kind)
	}
	return doc, nil
}

// printWorkflowRunResult renders steps as a compact, one-line-per-step
// log so the caller sees progress without opening the core UI. The
// order is the order the engine reported; with per-step timings it's
// easy to spot the slow step.
func printWorkflowRunResult(result corecli.WorkflowRunResult) {
	fmt.Printf("workflow status: %s\n", result.Status)
	for _, step := range result.Steps {
		marker := "✓"
		switch strings.ToLower(step.Status) {
		case "failed":
			marker = "✗"
		case "skipped":
			marker = "·"
		case "running":
			marker = "…"
		}
		line := fmt.Sprintf("  %s %-36s %s", marker, step.ID, step.Status)
		if step.Operation != "" {
			line += fmt.Sprintf("  (%s)", step.Operation)
		}
		if step.Attempts > 1 {
			line += fmt.Sprintf("  attempts=%d", step.Attempts)
		}
		if step.Error != "" {
			line += "  err=" + firstLine(step.Error)
		}
		fmt.Println(line)
	}
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

// clientFromFlags mirrors the pattern used by other subcommands:
// prefer explicit --server / --token, fall back to the active context
// in ~/.yggdrasil/config.yaml. Kept local so we don't grow a shared
// helper with one caller.
func clientFromFlags(server, token string) (*corecli.Client, error) {
	if server != "" {
		return corecli.NewClient(server, token), nil
	}
	client, err := corecli.FromContext("", token)
	if err != nil {
		return nil, err
	}
	return client, nil
}
