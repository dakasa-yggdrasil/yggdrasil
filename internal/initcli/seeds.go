package initcli

import (
	"context"
	"errors"
	"fmt"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

// defaultTopologyInstances returns the integration_instance manifests
// that match the standalone compose topology — two adapters reachable
// by service name on the compose network. These instances are
// deployment-specific (URLs reference compose service DNS) and
// therefore cannot live in yggdrasil-core's shipped seeds; the init
// command is the only place that knows the topology, so it seeds them
// post-boot.
//
// Idempotent: if an instance with the same (kind, namespace, name)
// already exists the HTTP API upserts a new version against the same
// logical record — re-running `yggdrasil init` stays safe.
func defaultTopologyInstances() []map[string]any {
	return []map[string]any{
		kubernetesInstance(),
		schemaMigrationsInstance(),
	}
}

// kubernetesInstance points at the integration-kubernetes adapter
// shipped in the standalone compose (HTTP transport, port 8081,
// reachable by service name inside the compose network).
//
// in_cluster=false tells the adapter to use the mounted ~/.kube/config
// volume instead of a service-account token — this is the right
// default for a developer laptop; production clusters rewrite this
// instance with their own credentials_ref.
func kubernetesInstance() map[string]any {
	return map[string]any{
		"apiVersion": "yggdrasil.io/v1alpha1",
		"kind":       "integration_instance",
		"metadata": map[string]any{
			"name":      "yggdrasil-core-kubernetes",
			"namespace": "global",
			"description": "Kubernetes integration instance pointing at the adapter service on the standalone compose network.",
			"labels": map[string]any{
				"bootstrap_source":              "yggdrasil-init",
				"yggdrasil.io/compose-topology": "standalone",
			},
		},
		"spec": map[string]any{
			"type_ref": map[string]any{
				"namespace": "global",
				"name":      "kubernetes",
			},
			"config": map[string]any{
				// base_url is the convention http_json-transport
				// adapters read to build their endpoint URL; it's a
				// standard config field, not a custom one.
				"base_url":          "http://integration-kubernetes:8081",
				"in_cluster":        false,
				"kubeconfig_path":   "/root/.kube/config",
				"default_namespace": "default",
				"field_manager":     "yggdrasil",
			},
		},
	}
}

// schemaMigrationsInstance points at the integration-schema-migrations
// adapter (HTTP port 8082 inside the compose network).
func schemaMigrationsInstance() map[string]any {
	return map[string]any{
		"apiVersion": "yggdrasil.io/v1alpha1",
		"kind":       "integration_instance",
		"metadata": map[string]any{
			"name":      "yggdrasil-core-schema-migrations",
			"namespace": "global",
			"description": "Schema-migrations (goose-postgres) adapter instance reachable on the standalone compose network.",
			"labels": map[string]any{
				"bootstrap_source":              "yggdrasil-init",
				"yggdrasil.io/compose-topology": "standalone",
			},
		},
		"spec": map[string]any{
			"type_ref": map[string]any{
				"namespace": "global",
				"name":      "schema-migrations-goose-postgres",
			},
			"config": map[string]any{
				"base_url": "http://integration-schema-migrations:8082",
			},
		},
	}
}

// applyTopologyInstances POSTs each default instance to the core's
// HTTP API as an authenticated admin. Swallowing errors partially
// (continuing past one failed apply) is NOT desired — missing an
// instance silently would yield a mysterious "no instance" error the
// first time a workflow tries to dispatch, so we fail fast and let
// the caller surface a diagnostic.
//
// The manifest API is flat: the server's payload struct has name,
// namespace, description, labels, spec at the top level (not a nested
// metadata block). We shape the POST body to match.
func applyTopologyInstances(ctx context.Context, server, token string) error {
	client := corecli.NewClient(server, token)
	for _, instance := range defaultTopologyInstances() {
		metadata, _ := instance["metadata"].(map[string]any)
		name, _ := metadata["name"].(string)
		namespace, _ := metadata["namespace"].(string)
		description, _ := metadata["description"].(string)
		rawLabels, _ := metadata["labels"].(map[string]any)
		labels := make(map[string]string, len(rawLabels))
		for k, v := range rawLabels {
			if s, ok := v.(string); ok {
				labels[k] = s
			}
		}
		payload := map[string]any{
			"name":      name,
			"namespace": namespace,
			"spec":      instance["spec"],
		}
		if description != "" {
			payload["description"] = description
		}
		if len(labels) > 0 {
			payload["labels"] = labels
		}
		if _, err := client.CreateManifest(ctx, "integration_instance", payload); err != nil {
			return fmt.Errorf("apply integration_instance %q: %w", name, shortenCoreError(err))
		}
	}
	return nil
}

// shortenCoreError strips the long multi-line trace core errors carry
// when they bubble up through corecli.Do. The CLI's banner stays
// readable; the structured error is still logged by the core.
func shortenCoreError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if idx := findNewline(msg); idx > 0 {
		return errors.New(msg[:idx])
	}
	return err
}

func findNewline(s string) int {
	for i, r := range s {
		if r == '\n' {
			return i
		}
	}
	return -1
}
