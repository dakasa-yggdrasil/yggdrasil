package message

import (
	"strings"
	"testing"
	"time"

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

func TestHeimdallEvaluateMemoryObservationTracksRecoveryTimingAndStability(t *testing.T) {
	now := time.Now().UTC()
	spec := model.GuardianMemoryManifestSpec{
		Status:             model.GuardianMemoryStatusObservedRecovered,
		ComponentKind:      "integration",
		ComponentNamespace: "global",
		ComponentName:      "heimdall-guardian",
		Execution: model.GuardianMemoryExecutionSpec{
			AttemptedAt: now.Add(-26 * time.Hour).Format(time.RFC3339),
			CompletedAt: now.Add(-25 * time.Hour).Format(time.RFC3339),
		},
		Observation: model.GuardianMemoryObservationSpec{
			ObservedAt:       now.Add(-24 * time.Hour).Format(time.RFC3339),
			ObservationCount: 1,
		},
	}

	status, observation, ok := heimdallEvaluateMemoryObservation(map[string]any{
		"integrations": []map[string]any{
			{
				"name":           "heimdall-guardian",
				"namespace":      "global",
				"overall_health": "healthy",
			},
		},
		"incidents": []map[string]any{},
	}, spec)
	if !ok {
		t.Fatal("expected observation to be evaluated")
	}
	if status != model.GuardianMemoryStatusObservedRecovered {
		t.Fatalf("status = %q, want observed_recovered", status)
	}
	if observation.TimeToRecoverySeconds <= 0 {
		t.Fatalf("time_to_recovery_seconds = %d, want > 0", observation.TimeToRecoverySeconds)
	}
	if observation.StableWindowSeconds < 24*60*60-60 {
		t.Fatalf("stable_window_seconds = %d, want roughly >= 24h", observation.StableWindowSeconds)
	}
	if observation.ObservationCount < 2 {
		t.Fatalf("observation_count = %d, want incremented count", observation.ObservationCount)
	}
}

func TestHeimdallDecisionFromAssessmentAutoExecutesTrustedPlaybooks(t *testing.T) {
	decision := heimdallDecisionFromAssessment(
		map[string]any{
			"type":              "dispatch_workflow",
			"incident_severity": "critical",
			"blast_radius":      "medium",
		},
		model.GuardianPolicyManifestSpec{
			Autonomy: model.GuardianAutonomyPolicySpec{
				Mode:                        "policy_bound",
				AutoExecuteMinConfidence:    0.7,
				ManualReviewBelowConfidence: 0.25,
				HotfixSeverityThreshold:     "critical",
				MaxAutoExecuteBlastRadius:   "medium",
				MaxBypassHotfixBlastRadius:  "high",
			},
		},
		"critical_auto_remediation",
		heimdallActionConfidenceAssessment{
			Confidence:       0.88,
			ConfidenceBand:   "trusted",
			ProviderGroup:    "kubernetes",
			IncidentCategory: "capacity",
			Attempts:         4,
			RecoveryRate:     0.92,
		},
	)

	if decision.RequireApproval {
		t.Fatal("expected trusted playbook to auto-execute")
	}
	if decision.ConfidenceBand != "trusted" {
		t.Fatalf("confidence band = %q, want trusted", decision.ConfidenceBand)
	}
}

func TestHeimdallDecisionFromAssessmentRequiresManualReviewForLowConfidence(t *testing.T) {
	decision := heimdallDecisionFromAssessment(
		map[string]any{
			"type":              "rightsize_component",
			"incident_severity": "critical",
		},
		model.GuardianPolicyManifestSpec{
			Autonomy: model.GuardianAutonomyPolicySpec{
				Mode:                        "policy_bound",
				AutoExecuteMinConfidence:    0.7,
				ManualReviewBelowConfidence: 0.25,
				HotfixSeverityThreshold:     "critical",
				MaxAutoExecuteBlastRadius:   "medium",
				MaxBypassHotfixBlastRadius:  "high",
			},
		},
		"critical_auto_remediation",
		heimdallActionConfidenceAssessment{
			Confidence:       0.12,
			ProviderGroup:    "kubernetes",
			IncidentCategory: "capacity",
			Attempts:         1,
			RecoveryRate:     0.1,
		},
	)

	if !decision.RequireApproval {
		t.Fatal("expected low-confidence playbook to require approval")
	}
	if !decision.ManualReview {
		t.Fatal("expected low-confidence playbook to require manual review")
	}
	if decision.ConfidenceBand != "manual_review" {
		t.Fatalf("confidence band = %q, want manual_review", decision.ConfidenceBand)
	}
}

func TestHeimdallDecisionFromAssessmentRequiresApprovalForHighBlastRadius(t *testing.T) {
	decision := heimdallDecisionFromAssessment(
		map[string]any{
			"type":         "dispatch_workflow",
			"blast_radius": "high",
		},
		model.GuardianPolicyManifestSpec{
			Autonomy: model.GuardianAutonomyPolicySpec{
				Mode:                        "policy_bound",
				AutoExecuteMinConfidence:    0.7,
				ManualReviewBelowConfidence: 0.25,
				HotfixSeverityThreshold:     "critical",
				MaxAutoExecuteBlastRadius:   "medium",
				MaxBypassHotfixBlastRadius:  "high",
			},
		},
		"critical_auto_remediation",
		heimdallActionConfidenceAssessment{
			Confidence:       0.93,
			ProviderGroup:    "github",
			IncidentCategory: "runtime",
			Attempts:         6,
			RecoveryRate:     0.95,
		},
	)

	if !decision.RequireApproval {
		t.Fatal("expected high blast radius playbook to require approval")
	}
	if decision.ManualReview {
		t.Fatal("expected high blast radius playbook to require approval, not manual review")
	}
	if decision.BlastRadius != "high" {
		t.Fatalf("blast radius = %q, want high", decision.BlastRadius)
	}
}

func TestHeimdallDecisionFromAssessmentRequiresManualReviewForCriticalBlastRadius(t *testing.T) {
	decision := heimdallDecisionFromAssessment(
		map[string]any{
			"type":         "direct_push",
			"blast_radius": "critical",
		},
		model.GuardianPolicyManifestSpec{
			Autonomy: model.GuardianAutonomyPolicySpec{
				Mode:                        "policy_bound",
				AutoExecuteMinConfidence:    0.7,
				ManualReviewBelowConfidence: 0.25,
				HotfixSeverityThreshold:     "critical",
				MaxAutoExecuteBlastRadius:   "medium",
				MaxBypassHotfixBlastRadius:  "high",
			},
		},
		"critical_auto_remediation",
		heimdallActionConfidenceAssessment{
			Confidence:       0.97,
			ProviderGroup:    "github",
			IncidentCategory: "repository",
			Attempts:         3,
			RecoveryRate:     1,
		},
	)

	if !decision.RequireApproval {
		t.Fatal("expected critical blast radius playbook to require approval")
	}
	if !decision.ManualReview {
		t.Fatal("expected critical blast radius playbook to require manual review")
	}
	if decision.ConfidenceBand != "manual_review" {
		t.Fatalf("confidence band = %q, want manual_review", decision.ConfidenceBand)
	}
}

func TestHeimdallDecisionFromAssessmentProtectsProductionEnvironment(t *testing.T) {
	decision := heimdallDecisionFromAssessmentAt(
		map[string]any{
			"type":         "dispatch_workflow",
			"blast_radius": "medium",
			"environment":  "production",
		},
		model.GuardianPolicyManifestSpec{
			Autonomy: model.GuardianAutonomyPolicySpec{
				Mode:                        "policy_bound",
				AutoExecuteMinConfidence:    0.7,
				ManualReviewBelowConfidence: 0.25,
				HotfixSeverityThreshold:     "critical",
				MaxAutoExecuteBlastRadius:   "medium",
				MaxBypassHotfixBlastRadius:  "high",
				ProtectedEnvironments: model.GuardianProtectedEnvironmentPolicySpec{
					Environments:               []string{"production"},
					MaxAutoExecuteBlastRadius:  "low",
					MaxBypassHotfixBlastRadius: "medium",
				},
			},
		},
		"critical_auto_remediation",
		heimdallActionConfidenceAssessment{
			Confidence:       0.95,
			ProviderGroup:    "github",
			IncidentCategory: "runtime",
			Attempts:         5,
			RecoveryRate:     1,
			BlastRadius:      "medium",
			Environment:      "production",
		},
		time.Date(2026, time.April, 6, 14, 0, 0, 0, time.UTC),
	)

	if !decision.RequireApproval {
		t.Fatal("expected production protection to require approval")
	}
	if !decision.ProtectedEnvironment {
		t.Fatal("expected production environment to be marked as protected")
	}
}

func TestHeimdallDecisionFromAssessmentBlocksOutsideBusinessHours(t *testing.T) {
	decision := heimdallDecisionFromAssessmentAt(
		map[string]any{
			"type":         "dispatch_workflow",
			"blast_radius": "low",
			"environment":  "production",
		},
		model.GuardianPolicyManifestSpec{
			Autonomy: model.GuardianAutonomyPolicySpec{
				Mode:                        "policy_bound",
				AutoExecuteMinConfidence:    0.7,
				ManualReviewBelowConfidence: 0.25,
				MaxAutoExecuteBlastRadius:   "medium",
				MaxBypassHotfixBlastRadius:  "high",
				BusinessHours: model.GuardianBusinessHoursPolicySpec{
					Enabled:           true,
					Timezone:          "UTC",
					Weekdays:          []string{"mon", "tue", "wed", "thu", "fri"},
					StartHour:         9,
					EndHour:           18,
					Environments:      []string{"production"},
					AllowHotfixBypass: false,
				},
			},
		},
		"critical_auto_remediation",
		heimdallActionConfidenceAssessment{
			Confidence:  0.95,
			BlastRadius: "low",
			Environment: "production",
		},
		time.Date(2026, time.April, 6, 2, 0, 0, 0, time.UTC),
	)

	if !decision.RequireApproval {
		t.Fatal("expected outside business hours to require approval")
	}
	if !decision.OutsideBusinessHours {
		t.Fatal("expected decision to flag outside business hours")
	}
}

func TestHeimdallDecisionFromAssessmentBlocksDuringFreezeWindow(t *testing.T) {
	decision := heimdallDecisionFromAssessmentAt(
		map[string]any{
			"type":         "dispatch_workflow",
			"blast_radius": "low",
			"environment":  "production",
		},
		model.GuardianPolicyManifestSpec{
			Autonomy: model.GuardianAutonomyPolicySpec{
				Mode:                        "policy_bound",
				AutoExecuteMinConfidence:    0.7,
				ManualReviewBelowConfidence: 0.25,
				MaxAutoExecuteBlastRadius:   "medium",
				MaxBypassHotfixBlastRadius:  "high",
				FreezeWindows: []model.GuardianFreezeWindowPolicySpec{
					{
						Name:         "release-freeze",
						StartsAt:     "2026-04-01T00:00:00Z",
						EndsAt:       "2026-04-10T00:00:00Z",
						Environments: []string{"production"},
					},
				},
			},
		},
		"critical_auto_remediation",
		heimdallActionConfidenceAssessment{
			Confidence:  0.95,
			BlastRadius: "low",
			Environment: "production",
		},
		time.Date(2026, time.April, 6, 14, 0, 0, 0, time.UTC),
	)

	if !decision.RequireApproval {
		t.Fatal("expected freeze window to require approval")
	}
	if !decision.ManualReview {
		t.Fatal("expected freeze window to require manual review")
	}
	if decision.ActiveFreezeWindow != "release-freeze" {
		t.Fatalf("freeze window = %q, want release-freeze", decision.ActiveFreezeWindow)
	}
}
