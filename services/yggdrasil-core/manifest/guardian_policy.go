package manifest

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/dakasa-co/yggdrasil-core/model"
)

var supportedGuardianSeverityThresholds = []string{"low", "medium", "high", "critical"}
var supportedGuardianAutonomyModes = []string{"policy_bound", "approval_required", "bypass_hotfix"}

// ParseGuardianPolicySpec parses the raw spec payload into the typed manifest.
func ParseGuardianPolicySpec(raw json.RawMessage) (model.GuardianPolicyManifestSpec, error) {
	var spec model.GuardianPolicyManifestSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return model.GuardianPolicyManifestSpec{}, fmt.Errorf("parse guardian_policy spec: %w", err)
	}
	return spec, nil
}

// ValidateGuardianPolicySpec validates one guardian policy manifest.
func ValidateGuardianPolicySpec(spec model.GuardianPolicyManifestSpec) error {
	if !guardianPolicySelectorProvided(spec.GuardianRef) {
		return fmt.Errorf("guardian_policy guardian_ref is required")
	}

	spec = NormalizeGuardianPolicySpec(spec)
	if !slices.Contains(supportedGuardianSeverityThresholds, spec.AutoHeal.SeverityThreshold) {
		return fmt.Errorf("guardian_policy auto_heal severity_threshold %q is unsupported", spec.AutoHeal.SeverityThreshold)
	}
	if !slices.Contains(supportedGuardianAutonomyModes, spec.Autonomy.Mode) {
		return fmt.Errorf("guardian_policy autonomy mode %q is unsupported", spec.Autonomy.Mode)
	}
	if !slices.Contains(supportedGuardianSeverityThresholds, spec.Autonomy.HotfixSeverityThreshold) {
		return fmt.Errorf("guardian_policy autonomy hotfix_severity_threshold %q is unsupported", spec.Autonomy.HotfixSeverityThreshold)
	}
	if spec.Autonomy.AutoExecuteMinConfidence < 0 || spec.Autonomy.AutoExecuteMinConfidence > 1 {
		return fmt.Errorf("guardian_policy autonomy auto_execute_min_confidence must be between 0 and 1")
	}
	if spec.Autonomy.ManualReviewBelowConfidence < 0 || spec.Autonomy.ManualReviewBelowConfidence > 1 {
		return fmt.Errorf("guardian_policy autonomy manual_review_below_confidence must be between 0 and 1")
	}
	if spec.Autonomy.ManualReviewBelowConfidence > spec.Autonomy.AutoExecuteMinConfidence {
		return fmt.Errorf("guardian_policy autonomy manual_review_below_confidence must be <= auto_execute_min_confidence")
	}
	if spec.AutoHeal.MaxActionsPerSweep < 0 {
		return fmt.Errorf("guardian_policy auto_heal max_actions_per_sweep must be >= 0")
	}
	if spec.AutoHeal.CooldownSeconds < 0 {
		return fmt.Errorf("guardian_policy auto_heal cooldown_seconds must be >= 0")
	}
	if spec.CostOptimization.MinEstimatedMonthlySavingsUSD < 0 {
		return fmt.Errorf("guardian_policy cost_optimization min_estimated_monthly_savings_usd must be >= 0")
	}
	return nil
}

// NormalizeGuardianPolicySpec applies compatibility defaults to guardian policy specs.
func NormalizeGuardianPolicySpec(spec model.GuardianPolicyManifestSpec) model.GuardianPolicyManifestSpec {
	spec.Scope = strings.TrimSpace(spec.Scope)
	if spec.Scope == "" {
		spec.Scope = "global"
	}

	spec.AutoHeal.SeverityThreshold = strings.ToLower(strings.TrimSpace(spec.AutoHeal.SeverityThreshold))
	if spec.AutoHeal.SeverityThreshold == "" {
		spec.AutoHeal.SeverityThreshold = "critical"
	}
	if spec.AutoHeal.MaxActionsPerSweep <= 0 {
		spec.AutoHeal.MaxActionsPerSweep = 1
	}
	if spec.AutoHeal.CooldownSeconds < 0 {
		spec.AutoHeal.CooldownSeconds = 0
	}
	if spec.AutoHeal.CooldownSeconds == 0 {
		spec.AutoHeal.CooldownSeconds = 300
	}

	spec.Autonomy.Mode = strings.ToLower(strings.TrimSpace(spec.Autonomy.Mode))
	if spec.Autonomy.Mode == "" {
		spec.Autonomy.Mode = "policy_bound"
	}
	spec.Autonomy.HotfixSeverityThreshold = strings.ToLower(strings.TrimSpace(spec.Autonomy.HotfixSeverityThreshold))
	if spec.Autonomy.HotfixSeverityThreshold == "" {
		spec.Autonomy.HotfixSeverityThreshold = "critical"
	}
	if spec.Autonomy.AutoExecuteMinConfidence <= 0 {
		spec.Autonomy.AutoExecuteMinConfidence = 0.7
	}
	if spec.Autonomy.ManualReviewBelowConfidence < 0 {
		spec.Autonomy.ManualReviewBelowConfidence = 0
	}
	if spec.Autonomy.ManualReviewBelowConfidence == 0 {
		spec.Autonomy.ManualReviewBelowConfidence = 0.25
	}
	if spec.Autonomy.ManualReviewBelowConfidence > spec.Autonomy.AutoExecuteMinConfidence {
		spec.Autonomy.ManualReviewBelowConfidence = spec.Autonomy.AutoExecuteMinConfidence
	}

	if spec.CostOptimization.MinEstimatedMonthlySavingsUSD < 0 {
		spec.CostOptimization.MinEstimatedMonthlySavingsUSD = 0
	}
	return spec
}

func guardianPolicySelectorProvided(selector model.ManifestSelector) bool {
	return strings.TrimSpace(selector.ManifestID) != "" ||
		strings.TrimSpace(selector.Namespace) != "" ||
		strings.TrimSpace(selector.Name) != "" ||
		selector.Version != nil
}
