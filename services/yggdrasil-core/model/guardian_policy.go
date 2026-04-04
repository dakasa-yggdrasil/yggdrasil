package model

// GuardianPolicyManifestSpec defines the autonomy and blast-radius limits for one guardian integration.
type GuardianPolicyManifestSpec struct {
	GuardianRef          ManifestSelector                       `json:"guardian_ref"`
	Scope                string                                 `json:"scope,omitempty"`
	AutoHeal             GuardianAutoHealPolicySpec             `json:"auto_heal,omitempty"`
	Autonomy             GuardianAutonomyPolicySpec             `json:"autonomy,omitempty"`
	RepositoryAutomation GuardianRepositoryAutomationPolicySpec `json:"repository_automation,omitempty"`
	CostOptimization     GuardianCostOptimizationPolicySpec     `json:"cost_optimization,omitempty"`
}

// GuardianAutoHealPolicySpec constrains automatic runtime remediation.
type GuardianAutoHealPolicySpec struct {
	Enabled               bool   `json:"enabled,omitempty"`
	SeverityThreshold     string `json:"severity_threshold,omitempty"`
	MaxActionsPerSweep    int    `json:"max_actions_per_sweep,omitempty"`
	CooldownSeconds       int    `json:"cooldown_seconds,omitempty"`
	AllowDispatchWorkflow bool   `json:"allow_dispatch_workflow,omitempty"`
	AllowRotateSecret     bool   `json:"allow_rotate_secret,omitempty"`
	AllowRightsize        bool   `json:"allow_rightsize,omitempty"`
}

// GuardianRepositoryAutomationPolicySpec constrains repository-facing changes.
type GuardianRepositoryAutomationPolicySpec struct {
	AllowPullRequestAutomation bool `json:"allow_pull_request_automation,omitempty"`
	AllowDirectPush            bool `json:"allow_direct_push,omitempty"`
}

// GuardianCostOptimizationPolicySpec constrains proactive cost actions.
type GuardianCostOptimizationPolicySpec struct {
	Enabled                       bool    `json:"enabled,omitempty"`
	MinEstimatedMonthlySavingsUSD float64 `json:"min_estimated_monthly_savings_usd,omitempty"`
	AllowRightsize                bool    `json:"allow_rightsize,omitempty"`
}

// GuardianAutonomyPolicySpec controls when Heimdall may act directly, when it
// must request approval, and when it may engage the LLM fallback path.
type GuardianAutonomyPolicySpec struct {
	Mode                        string  `json:"mode,omitempty"`
	AllowLLMFallback            bool    `json:"allow_llm_fallback,omitempty"`
	HotfixSeverityThreshold     string  `json:"hotfix_severity_threshold,omitempty"`
	AutoExecuteMinConfidence    float64 `json:"auto_execute_min_confidence,omitempty"`
	ManualReviewBelowConfidence float64 `json:"manual_review_below_confidence,omitempty"`
	MaxAutoExecuteBlastRadius   string  `json:"max_auto_execute_blast_radius,omitempty"`
	MaxBypassHotfixBlastRadius  string  `json:"max_bypass_hotfix_blast_radius,omitempty"`
}
