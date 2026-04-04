package manifest

import (
	"encoding/json"
	"testing"

	"github.com/dakasa-co/yggdrasil-core/model"
)

func TestValidateGuardianPolicySpec(t *testing.T) {
	spec := guardianPolicyFixture()

	if err := ValidateGuardianPolicySpec(spec); err != nil {
		t.Fatalf("ValidateGuardianPolicySpec error: %v", err)
	}
}

func TestValidateGuardianPolicySpecRejectsMissingGuardianRef(t *testing.T) {
	spec := guardianPolicyFixture()
	spec.GuardianRef = model.ManifestSelector{}

	if err := ValidateGuardianPolicySpec(spec); err == nil {
		t.Fatal("expected missing guardian_ref to fail validation")
	}
}

func TestGuardianPolicyDocumentValidation(t *testing.T) {
	raw, err := json.Marshal(guardianPolicyFixture())
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	doc := model.ManifestDocument{
		APIVersion: "yggdrasil.io/v1alpha1",
		Kind:       "guardian_policy",
		Metadata: model.ManifestMetadataInput{
			Name:      "heimdall-default",
			Namespace: "global",
		},
		Spec: raw,
	}

	if err := ValidateDocument(doc); err != nil {
		t.Fatalf("ValidateDocument(guardian_policy) error: %v", err)
	}
}

func guardianPolicyFixture() model.GuardianPolicyManifestSpec {
	return model.GuardianPolicyManifestSpec{
		GuardianRef: model.ManifestSelector{
			Namespace: "global",
			Name:      "heimdall-guardian",
		},
		Scope: "global",
		AutoHeal: model.GuardianAutoHealPolicySpec{
			Enabled:               true,
			SeverityThreshold:     "critical",
			MaxActionsPerSweep:    2,
			CooldownSeconds:       300,
			AllowDispatchWorkflow: true,
		},
		CostOptimization: model.GuardianCostOptimizationPolicySpec{
			Enabled:                       true,
			MinEstimatedMonthlySavingsUSD: 50,
		},
		Autonomy: model.GuardianAutonomyPolicySpec{
			Mode:                        "policy_bound",
			AllowLLMFallback:            true,
			HotfixSeverityThreshold:     "critical",
			AutoExecuteMinConfidence:    0.7,
			ManualReviewBelowConfidence: 0.25,
			MaxAutoExecuteBlastRadius:   "medium",
			MaxBypassHotfixBlastRadius:  "high",
		},
	}
}

func TestValidateGuardianPolicySpecRejectsInvalidConfidenceThresholds(t *testing.T) {
	spec := guardianPolicyFixture()
	spec.Autonomy.AutoExecuteMinConfidence = 1.2

	if err := ValidateGuardianPolicySpec(spec); err == nil {
		t.Fatal("expected invalid auto_execute_min_confidence to fail validation")
	}
}

func TestValidateGuardianPolicySpecRejectsInvalidBlastRadiusOrder(t *testing.T) {
	spec := guardianPolicyFixture()
	spec.Autonomy.MaxAutoExecuteBlastRadius = "critical"
	spec.Autonomy.MaxBypassHotfixBlastRadius = "medium"

	if err := ValidateGuardianPolicySpec(spec); err == nil {
		t.Fatal("expected invalid blast radius ordering to fail validation")
	}
}
