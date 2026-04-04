package message

import (
	"strings"
	"testing"

	"github.com/dakasa-co/yggdrasil-core/model"
)

func TestHeimdallIntegrationIncidentsAddsOOMCapacityIncident(t *testing.T) {
	health := model.IntegrationInstanceHealth{
		IntegrationInstance: model.ManifestReference{
			Namespace: "global",
			Name:      "heimdall-guardian",
		},
		IntegrationType: model.ManifestReference{
			Namespace: "global",
			Name:      "heimdall",
		},
		RuntimeState: &model.IntegrationRuntimeState{
			Status:  model.IntegrationRuntimeStatusUnreachable,
			Message: "container exited with OOMKilled",
			Details: map[string]any{
				"oom_killed": true,
			},
		},
	}

	incidents := heimdallIntegrationIncidents(health, map[string]any{
		"repository": "dakasa-yggdrasil/integration-heimdall",
	})
	if len(incidents) != 2 {
		t.Fatalf("incidents = %d, want 2", len(incidents))
	}

	foundCapacity := false
	for _, incident := range incidents {
		if strings.EqualFold(anyString(incident["category"]), "capacity") {
			foundCapacity = true
			break
		}
	}
	if !foundCapacity {
		t.Fatal("expected capacity incident for oom-killed runtime")
	}
}

func TestHeimdallIntegrationIncidentsEnrichEvidenceWithProviderContext(t *testing.T) {
	health := model.IntegrationInstanceHealth{
		IntegrationInstance: model.ManifestReference{
			Namespace: "global",
			Name:      "integration-github-prod",
		},
		IntegrationType: model.ManifestReference{
			Namespace: "global",
			Name:      "github",
		},
		RuntimeState: &model.IntegrationRuntimeState{
			Status:  model.IntegrationRuntimeStatusUnreachable,
			Message: "adapter timed out",
			Details: map[string]any{
				"restart_count": 4,
			},
		},
	}

	incidents := heimdallIntegrationIncidents(health, map[string]any{
		"repository":            "dakasa-yggdrasil/integration-github",
		"guardian_support_mode": "lightweight",
	})
	if len(incidents) == 0 {
		t.Fatal("incidents = 0, want at least one")
	}

	evidence, ok := incidents[0]["evidence"].(map[string]any)
	if !ok {
		t.Fatalf("incident evidence type = %T, want map[string]any", incidents[0]["evidence"])
	}
	if got := anyString(evidence["type_name"]); got != "github" {
		t.Fatalf("type_name = %q, want github", got)
	}
	if got := anyString(evidence["type_namespace"]); got != "global" {
		t.Fatalf("type_namespace = %q, want global", got)
	}
	if got := anyString(evidence["repository"]); got != "dakasa-yggdrasil/integration-github" {
		t.Fatalf("repository = %q, want dakasa-yggdrasil/integration-github", got)
	}
}

func TestResolveHeimdallContractAction(t *testing.T) {
	contracts := map[string]heimdallRemediationContract{
		heimdallComponentKey("surface", "global", "yggdrasil-console"): {
			Spec: model.RemediationContractManifestSpec{
				ComponentKind:      "surface",
				ComponentNamespace: "global",
				ComponentName:      "yggdrasil-console",
				Actions: []model.RemediationContractActionSpec{
					{
						Name:        "rightsize_component",
						Mode:        model.RemediationContractActionModeWorkflowDispatch,
						AutoExecute: true,
						WorkflowDispatch: &model.RemediationWorkflowDispatchSpec{
							Workflow: "deploy.yml",
							Ref:      "main",
						},
					},
				},
			},
		},
	}

	contract, action, err := resolveHeimdallContractAction(map[string]any{
		"type":                "rightsize_component",
		"component_kind":      "surface",
		"component_namespace": "global",
		"component_name":      "yggdrasil-console",
	}, contracts)
	if err != nil {
		t.Fatalf("resolveHeimdallContractAction error: %v", err)
	}
	if contract.Spec.ComponentName != "yggdrasil-console" {
		t.Fatalf("contract component_name = %q, want yggdrasil-console", contract.Spec.ComponentName)
	}
	if action.Name != "rightsize_component" {
		t.Fatalf("action name = %q, want rightsize_component", action.Name)
	}
}

func TestHeimdallBuildWorkflowInputsIncludesRemediationFields(t *testing.T) {
	inputs := heimdallBuildWorkflowInputs(
		map[string]any{
			"type":                "rightsize_component",
			"component_kind":      "integration",
			"component_namespace": "global",
			"component_name":      "heimdall-guardian",
			"reason":              "oom recovery",
			"workflow": map[string]any{
				"inputs": map[string]any{
					"incident_title": "Integration was OOMKilled",
				},
			},
		},
		model.RepositoryBindingManifestSpec{
			ComponentKind:      "integration",
			ComponentNamespace: "global",
			ComponentName:      "heimdall-guardian",
			Metadata: map[string]any{
				"environment": "production",
			},
		},
		"critical_auto_remediation",
		map[string]any{
			"remediation_contract": "heimdall.rightsize.v1",
		},
	)

	if got := anyString(inputs["component_namespace"]); got != "global" {
		t.Fatalf("component_namespace = %q, want global", got)
	}
	if got := anyString(inputs["incident_title"]); got != "Integration was OOMKilled" {
		t.Fatalf("incident_title = %q, want propagated incident title", got)
	}
	if got := anyString(inputs["remediation_contract"]); got != "heimdall.rightsize.v1" {
		t.Fatalf("remediation_contract = %q, want heimdall.rightsize.v1", got)
	}
	if got := anyString(inputs["event_name"]); got != "critical_auto_remediation" {
		t.Fatalf("event_name = %q, want critical_auto_remediation", got)
	}
}
