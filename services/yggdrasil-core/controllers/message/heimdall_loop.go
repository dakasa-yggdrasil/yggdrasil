package message

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	manifestengine "github.com/dakasa-co/yggdrasil-core/manifest"
	"github.com/dakasa-co/yggdrasil-core/model"
	"github.com/dakasa-co/yggdrasil-core/repository"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const (
	defaultHeimdallGuardianLoopInterval = 2 * time.Minute
	defaultHeimdallGuardianNamespace    = "global"
	defaultHeimdallGuardianInstance     = "heimdall-guardian"
	defaultHeimdallDispatchEnvironment  = "production"
	defaultHeimdallDispatchActor        = "heimdall"
	defaultHeimdallDispatchWorkflow     = "deploy.yml"
	defaultHeimdallDispatchBranch       = "main"
)

var (
	heimdallActionCooldownMu sync.Mutex
	heimdallActionCooldown   = map[string]time.Time{}
)

type heimdallRepositoryBinding struct {
	Manifest model.Manifest
	Spec     model.RepositoryBindingManifestSpec
}

type heimdallRemediationContract struct {
	Manifest model.Manifest
	Spec     model.RemediationContractManifestSpec
}

// StartHeimdallGuardianLoop runs the periodic closed-loop guardian sweep.
func StartHeimdallGuardianLoop(
	conn *amqp.Connection,
	db *sql.DB,
	logger *zap.Logger,
	interval time.Duration,
) context.CancelFunc {
	if interval <= 0 {
		interval = defaultHeimdallGuardianLoopInterval
	}

	ctx, cancel := context.WithCancel(context.Background())
	go runHeimdallGuardianLoop(ctx, conn, db, logger, interval)
	return cancel
}

func runHeimdallGuardianLoop(
	ctx context.Context,
	conn *amqp.Connection,
	db *sql.DB,
	logger *zap.Logger,
	interval time.Duration,
) {
	runHeimdallGuardianSweep(ctx, conn, db, logger)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runHeimdallGuardianSweep(ctx, conn, db, logger)
		}
	}
}

func runHeimdallGuardianSweep(
	ctx context.Context,
	conn *amqp.Connection,
	db *sql.DB,
	logger *zap.Logger,
) {
	if conn == nil || db == nil {
		return
	}

	instanceManifest, instanceSpec, typeManifest, typeSpec, err := resolveIntegrationInstance(ctx, conn, db, model.ManifestSelector{
		Namespace: defaultHeimdallGuardianNamespace,
		Name:      defaultHeimdallGuardianInstance,
	})
	if err != nil {
		if logger != nil {
			logger.Warn("heimdall guardian sweep skipped because the guardian instance is unavailable", zap.Error(err))
		}
		return
	}

	repositoryBindings, err := loadHeimdallRepositoryBindings(ctx, db)
	if err != nil {
		if logger != nil {
			logger.Warn("heimdall guardian sweep failed to load repository bindings", zap.Error(err))
		}
		return
	}

	policy, err := loadGuardianPolicyForInstance(ctx, db, instanceManifest)
	if err != nil {
		if logger != nil {
			logger.Warn("heimdall guardian sweep failed to load guardian policy", zap.Error(err))
		}
		return
	}

	remediationContracts, err := loadHeimdallRemediationContracts(ctx, db)
	if err != nil {
		if logger != nil {
			logger.Warn("heimdall guardian sweep failed to load remediation contracts", zap.Error(err))
		}
		return
	}

	snapshot, err := buildHeimdallEcosystemSnapshot(ctx, db, repositoryBindings)
	if err != nil {
		if logger != nil {
			logger.Warn("heimdall guardian sweep failed to build ecosystem snapshot", zap.Error(err))
		}
		return
	}

	assessmentResponse, err := executeIntegrationThroughResolved(
		ctx,
		conn,
		model.ExecuteIntegrationRequest{
			Operation:  "assess_ecosystem",
			Capability: "assess_ecosystem",
			Input: map[string]any{
				"ecosystem": snapshot,
			},
			Metadata: map[string]any{
				"source": "core.heimdall.assess",
			},
		},
		instanceManifest,
		instanceSpec,
		typeManifest,
		typeSpec,
		0,
	)
	if err != nil {
		if logger != nil {
			logger.Warn("heimdall guardian assessment failed", zap.Error(err))
		}
		return
	}

	if logger != nil {
		logger.Info("heimdall guardian assessment completed",
			zap.Any("metadata", assessmentResponse.Metadata),
		)
	}

	recommendResponse, err := executeIntegrationThroughResolved(
		ctx,
		conn,
		model.ExecuteIntegrationRequest{
			Operation:  "recommend_improvements",
			Capability: "recommend_improvements",
			Input: map[string]any{
				"ecosystem": snapshot,
			},
			Metadata: map[string]any{
				"source": "core.heimdall.recommend",
			},
		},
		instanceManifest,
		instanceSpec,
		typeManifest,
		typeSpec,
		0,
	)
	if err != nil {
		if logger != nil {
			logger.Warn("heimdall guardian recommendation sweep failed", zap.Error(err))
		}
	} else if logger != nil {
		logger.Info("heimdall guardian recommendations prepared",
			zap.Any("metadata", recommendResponse.Metadata),
		)
	}

	actionsExecuted := 0
	if policy.AutoHeal.Enabled {
		autoHealResponse, err := executeIntegrationThroughResolved(
			ctx,
			conn,
			model.ExecuteIntegrationRequest{
				Operation:  "auto_remediate_critical",
				Capability: "auto_remediate_critical",
				Input: map[string]any{
					"ecosystem": snapshot,
				},
				Metadata: map[string]any{
					"source": "core.heimdall.auto_heal",
				},
			},
			instanceManifest,
			instanceSpec,
			typeManifest,
			typeSpec,
			0,
		)
		if err != nil {
			if logger != nil {
				logger.Warn("heimdall guardian auto-remediation sweep failed", zap.Error(err))
			}
		} else {
			if heimdallSeverityMeetsThreshold(
				heimdallOutputIncidentSeverity(autoHealResponse.Output),
				policy.AutoHeal.SeverityThreshold,
			) {
				actions := heimdallRemediationActions(autoHealResponse.Output)
				executed, execErr := executeHeimdallActions(
					ctx,
					conn,
					db,
					actions,
					repositoryBindings,
					remediationContracts,
					policy,
					"critical_auto_remediation",
					actionsExecuted,
				)
				actionsExecuted = executed
				if execErr != nil && logger != nil {
					logger.Warn("heimdall guardian auto-remediation execution failed", zap.Error(execErr))
				}
			} else if logger != nil {
				logger.Info("heimdall guardian skipped auto-remediation because the incident severity is below threshold")
			}
		}
	}

	if policy.CostOptimization.Enabled {
		costResponse, err := executeIntegrationThroughResolved(
			ctx,
			conn,
			model.ExecuteIntegrationRequest{
				Operation:  "optimize_cost",
				Capability: "optimize_cost",
				Input: map[string]any{
					"ecosystem": snapshot,
				},
				Metadata: map[string]any{
					"source": "core.heimdall.optimize_cost",
				},
			},
			instanceManifest,
			instanceSpec,
			typeManifest,
			typeSpec,
			0,
		)
		if err != nil {
			if logger != nil {
				logger.Warn("heimdall guardian cost optimization sweep failed", zap.Error(err))
			}
		} else {
			actions := heimdallCostActions(costResponse.Output, policy)
			executed, execErr := executeHeimdallActions(
				ctx,
				conn,
				db,
				actions,
				repositoryBindings,
				remediationContracts,
				policy,
				"cost_optimization",
				actionsExecuted,
			)
			actionsExecuted = executed
			if execErr != nil && logger != nil {
				logger.Warn("heimdall guardian cost action execution failed", zap.Error(execErr))
			}
		}
	}
}

func loadHeimdallRepositoryBindings(ctx context.Context, db *sql.DB) (map[string]heimdallRepositoryBinding, error) {
	manifests, err := repository.ListManifests(ctx, db, model.ListManifestFilters{
		Kind:       "repository_binding",
		ActiveOnly: true,
	})
	if err != nil {
		return nil, err
	}

	bindings := make(map[string]heimdallRepositoryBinding, len(manifests))
	for _, manifestRecord := range manifests {
		spec, err := manifestengine.ParseRepositoryBindingSpec(manifestRecord.Spec)
		if err != nil {
			return nil, err
		}
		spec = manifestengine.NormalizeRepositoryBindingSpec(spec)
		bindings[heimdallComponentKey(spec.ComponentKind, spec.ComponentNamespace, spec.ComponentName)] = heimdallRepositoryBinding{
			Manifest: manifestRecord,
			Spec:     spec,
		}
	}

	return bindings, nil
}

func loadGuardianPolicyForInstance(ctx context.Context, db *sql.DB, instanceManifest model.Manifest) (model.GuardianPolicyManifestSpec, error) {
	manifests, err := repository.ListManifests(ctx, db, model.ListManifestFilters{
		Kind:       "guardian_policy",
		ActiveOnly: true,
	})
	if err != nil {
		return model.GuardianPolicyManifestSpec{}, err
	}

	for _, manifestRecord := range manifests {
		spec, err := manifestengine.ParseGuardianPolicySpec(manifestRecord.Spec)
		if err != nil {
			return model.GuardianPolicyManifestSpec{}, err
		}
		spec = manifestengine.NormalizeGuardianPolicySpec(spec)
		if guardianPolicyTargetsInstance(spec, instanceManifest) {
			return spec, nil
		}
	}

	return manifestengine.NormalizeGuardianPolicySpec(model.GuardianPolicyManifestSpec{
		GuardianRef: model.ManifestSelector{
			Namespace: instanceManifest.Metadata.Namespace,
			Name:      instanceManifest.Metadata.Name,
		},
		AutoHeal: model.GuardianAutoHealPolicySpec{
			Enabled:               true,
			SeverityThreshold:     "critical",
			MaxActionsPerSweep:    1,
			CooldownSeconds:       300,
			AllowDispatchWorkflow: true,
			AllowRightsize:        true,
		},
		CostOptimization: model.GuardianCostOptimizationPolicySpec{
			Enabled: false,
		},
	}), nil
}

func loadHeimdallRemediationContracts(ctx context.Context, db *sql.DB) (map[string]heimdallRemediationContract, error) {
	manifests, err := repository.ListManifests(ctx, db, model.ListManifestFilters{
		Kind:       "remediation_contract",
		ActiveOnly: true,
	})
	if err != nil {
		return nil, err
	}

	contracts := make(map[string]heimdallRemediationContract, len(manifests))
	for _, manifestRecord := range manifests {
		spec, err := manifestengine.ParseRemediationContractSpec(manifestRecord.Spec)
		if err != nil {
			return nil, err
		}
		spec = manifestengine.NormalizeRemediationContractSpec(spec)
		contracts[heimdallComponentKey(spec.ComponentKind, spec.ComponentNamespace, spec.ComponentName)] = heimdallRemediationContract{
			Manifest: manifestRecord,
			Spec:     spec,
		}
	}

	return contracts, nil
}

func guardianPolicyTargetsInstance(spec model.GuardianPolicyManifestSpec, instanceManifest model.Manifest) bool {
	if manifestID := strings.TrimSpace(spec.GuardianRef.ManifestID); manifestID != "" {
		return manifestID == instanceManifest.ID.String()
	}

	namespace := strings.TrimSpace(spec.GuardianRef.Namespace)
	if namespace == "" {
		namespace = "global"
	}

	return strings.EqualFold(namespace, instanceManifest.Metadata.Namespace) &&
		strings.EqualFold(strings.TrimSpace(spec.GuardianRef.Name), instanceManifest.Metadata.Name)
}

func buildHeimdallEcosystemSnapshot(
	ctx context.Context,
	db *sql.DB,
	repositoryBindings map[string]heimdallRepositoryBinding,
) (map[string]any, error) {
	integrationManifests, err := repository.ListManifests(ctx, db, model.ListManifestFilters{
		Kind:       "integration_instance",
		ActiveOnly: true,
	})
	if err != nil {
		return nil, err
	}

	integrations := make([]map[string]any, 0, len(integrationManifests))
	incidents := make([]map[string]any, 0)
	for _, manifestRecord := range integrationManifests {
		health, err := buildIntegrationInstanceHealth(ctx, db, manifestRecord, model.IntegrationRuntimeCheckKindOverall)
		if err != nil {
			return nil, err
		}

		item := map[string]any{
			"name":            manifestRecord.Metadata.Name,
			"namespace":       manifestRecord.Metadata.Namespace,
			"declared_status": health.DeclaredStatus,
			"overall_health":  health.Status,
			"type_name":       health.IntegrationType.Name,
			"type_namespace":  health.IntegrationType.Namespace,
		}
		if binding, ok := repositoryBindings[heimdallComponentKey("integration", manifestRecord.Metadata.Namespace, manifestRecord.Metadata.Name)]; ok {
			item = mergeStringAnyMaps(item, heimdallRepositoryBindingMetadata(binding))
		}
		if representative := health.RuntimeState; representative != nil {
			item["check_kind"] = representative.CheckKind
			item["health_message"] = representative.Message
			item["details"] = representative.Details
			if representative.LastFailureAt != nil {
				item["last_failure_at"] = representative.LastFailureAt.UTC().Format(time.RFC3339)
			}
			if representative.LastSuccessAt != nil {
				item["last_success_at"] = representative.LastSuccessAt.UTC().Format(time.RFC3339)
			}
			if restartCount, ok := heimdallNumericDetail(representative.Details, "restart_count", "crash_loop_count"); ok {
				item["restart_count"] = restartCount
			}
			if errorRate, ok := heimdallNumericDetail(representative.Details, "error_rate", "failure_rate"); ok {
				item["error_rate"] = errorRate
			}
		}
		integrations = append(integrations, item)
		incidents = append(incidents, heimdallIntegrationIncidents(health, item)...)
	}

	surfaceManifests, err := repository.ListManifests(ctx, db, model.ListManifestFilters{
		Kind:       "surface",
		ActiveOnly: true,
	})
	if err != nil {
		return nil, err
	}

	surfaces := make([]map[string]any, 0, len(surfaceManifests))
	for _, manifestRecord := range surfaceManifests {
		spec, err := manifestengine.ParseSurfaceSpec(manifestRecord.Spec)
		if err != nil {
			return nil, err
		}
		item := map[string]any{
			"name":           manifestRecord.Metadata.Name,
			"namespace":      manifestRecord.Metadata.Namespace,
			"category":       spec.Category,
			"overall_health": "healthy",
			"runtime_kind":   spec.Runtime.Kind,
			"exposure":       spec.Runtime.Exposure,
		}
		if binding, ok := repositoryBindings[heimdallComponentKey("surface", manifestRecord.Metadata.Namespace, manifestRecord.Metadata.Name)]; ok {
			item = mergeStringAnyMaps(item, heimdallRepositoryBindingMetadata(binding))
		}
		surfaces = append(surfaces, item)
	}

	secretsRaw, err := repository.ListManagedSecrets(ctx, db, model.ListManagedSecretsRequest{})
	if err != nil {
		return nil, err
	}

	secrets := make([]map[string]any, 0, len(secretsRaw))
	now := time.Now().UTC()
	for _, secret := range secretsRaw {
		item := map[string]any{
			"name":              secret.Name,
			"namespace":         secret.Namespace,
			"status":            secret.Status,
			"rotation_required": secret.IsRotationDue(now),
		}
		if secret.ExpiresAt != nil {
			item["expires_at"] = secret.ExpiresAt.UTC().Format(time.RFC3339)
			item["expires_in_hours"] = secret.ExpiresAt.UTC().Sub(now).Hours()
		}
		secrets = append(secrets, item)

		switch strings.ToLower(strings.TrimSpace(secret.Status)) {
		case "disabled", "revoked":
			incidents = append(incidents, map[string]any{
				"severity":            "critical",
				"status":              "open",
				"category":            "secret",
				"title":               "Managed secret requires remediation",
				"message":             fmt.Sprintf("Managed secret %s/%s is %s.", secret.Namespace, secret.Name, secret.Status),
				"component_kind":      "secret",
				"component_name":      secret.Name,
				"component_namespace": secret.Namespace,
			})
		}
		if secret.IsExpired(now) {
			incidents = append(incidents, map[string]any{
				"severity":            "critical",
				"status":              "open",
				"category":            "secret",
				"title":               "Managed secret expired",
				"message":             fmt.Sprintf("Managed secret %s/%s expired.", secret.Namespace, secret.Name),
				"component_kind":      "secret",
				"component_name":      secret.Name,
				"component_namespace": secret.Namespace,
			})
		}
	}

	repositories := make([]map[string]any, 0, len(repositoryBindings))
	for _, binding := range repositoryBindings {
		repositories = append(repositories, map[string]any{
			"name":                          binding.Spec.ComponentName,
			"component_kind":                binding.Spec.ComponentKind,
			"component_namespace":           binding.Spec.ComponentNamespace,
			"component_name":                binding.Spec.ComponentName,
			"repository":                    binding.Spec.Repository,
			"default_branch":                binding.Spec.DefaultBranch,
			"deploy_workflow":               binding.Spec.DeployWorkflow,
			"observe":                       binding.Spec.Automation.Observe,
			"allow_dispatch_workflow":       binding.Spec.Automation.AllowDispatchWorkflow,
			"allow_pull_request_automation": binding.Spec.Automation.AllowPullRequestAutomation,
			"allow_direct_push":             binding.Spec.Automation.AllowDirectPush,
			"metadata":                      cloneAuthorizationInput(binding.Spec.Metadata),
		})
	}

	return map[string]any{
		"integrations": integrations,
		"surfaces":     surfaces,
		"secrets":      secrets,
		"incidents":    incidents,
		"signals":      []map[string]any{},
		"repositories": repositories,
		"metadata": map[string]any{
			"generated_at":           now.Format(time.RFC3339),
			"repository_bindings":    len(repositoryBindings),
			"default_guardian_scope": "global",
		},
	}, nil
}

func heimdallRepositoryBindingMetadata(binding heimdallRepositoryBinding) map[string]any {
	metadata := map[string]any{
		"repository":                    binding.Spec.Repository,
		"default_branch":                binding.Spec.DefaultBranch,
		"deploy_workflow":               binding.Spec.DeployWorkflow,
		"allow_dispatch_workflow":       binding.Spec.Automation.AllowDispatchWorkflow,
		"allow_direct_push":             binding.Spec.Automation.AllowDirectPush,
		"allow_pull_request_automation": binding.Spec.Automation.AllowPullRequestAutomation,
	}
	return mergeStringAnyMaps(metadata, cloneAuthorizationInput(binding.Spec.Metadata))
}

func heimdallIntegrationIncidents(health model.IntegrationInstanceHealth, item map[string]any) []map[string]any {
	if health.RuntimeState == nil {
		return nil
	}

	status := strings.ToLower(strings.TrimSpace(health.RuntimeState.Status))
	severity := ""
	category := ""
	switch status {
	case model.IntegrationRuntimeStatusUnreachable:
		severity = "critical"
		category = "availability"
	case model.IntegrationRuntimeStatusInvalidResponse:
		severity = "critical"
		category = "stability"
	case model.IntegrationRuntimeStatusContractMismatch:
		severity = "high"
		category = "contract"
	default:
		severity = ""
	}

	incidents := make([]map[string]any, 0, 2)
	if severity != "" {
		incidents = append(incidents, map[string]any{
			"severity":            severity,
			"status":              "open",
			"category":            category,
			"title":               fmt.Sprintf("Integration %s is %s", health.IntegrationInstance.Name, status),
			"message":             health.RuntimeState.Message,
			"component_kind":      "integration",
			"component_name":      health.IntegrationInstance.Name,
			"component_namespace": health.IntegrationInstance.Namespace,
			"repository":          anyString(item["repository"]),
			"evidence":            cloneAuthorizationInput(health.RuntimeState.Details),
		})
	}

	if heimdallRuntimeIndicatesOOM(health.RuntimeState) {
		incidents = append(incidents, map[string]any{
			"severity":            "critical",
			"status":              "open",
			"category":            "capacity",
			"title":               fmt.Sprintf("Integration %s was OOM killed", health.IntegrationInstance.Name),
			"message":             firstNonEmpty(health.RuntimeState.Message, "The runtime reported an OOM termination and likely needs a bounded memory increase."),
			"component_kind":      "integration",
			"component_name":      health.IntegrationInstance.Name,
			"component_namespace": health.IntegrationInstance.Namespace,
			"repository":          anyString(item["repository"]),
			"evidence":            cloneAuthorizationInput(health.RuntimeState.Details),
		})
	}

	return incidents
}

func executeHeimdallActions(
	ctx context.Context,
	conn *amqp.Connection,
	db *sql.DB,
	actions []map[string]any,
	repositoryBindings map[string]heimdallRepositoryBinding,
	remediationContracts map[string]heimdallRemediationContract,
	policy model.GuardianPolicyManifestSpec,
	source string,
	executedSoFar int,
) (int, error) {
	executed := executedSoFar
	var firstErr error
	for _, action := range actions {
		if executed >= policy.AutoHeal.MaxActionsPerSweep {
			break
		}
		if err := executeHeimdallAction(ctx, conn, db, action, repositoryBindings, remediationContracts, policy, source); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		executed++
	}
	return executed, firstErr
}

func executeHeimdallAction(
	ctx context.Context,
	conn *amqp.Connection,
	db *sql.DB,
	action map[string]any,
	repositoryBindings map[string]heimdallRepositoryBinding,
	remediationContracts map[string]heimdallRemediationContract,
	policy model.GuardianPolicyManifestSpec,
	source string,
) error {
	actionType := strings.ToLower(strings.TrimSpace(anyString(action["type"])))
	switch actionType {
	case "dispatch_workflow":
		if !policy.AutoHeal.AllowDispatchWorkflow {
			return fmt.Errorf("heimdall guardian policy blocks dispatch_workflow actions")
		}
		return executeHeimdallWorkflowDispatch(ctx, conn, db, action, repositoryBindings, policy, source)
	case "rightsize_component":
		allowRightsize := policy.AutoHeal.AllowRightsize
		if source == "cost_optimization" {
			allowRightsize = policy.CostOptimization.AllowRightsize
		}
		if !allowRightsize {
			return fmt.Errorf("heimdall guardian policy blocks rightsize_component actions")
		}
		return executeHeimdallContractAction(ctx, conn, db, action, repositoryBindings, remediationContracts, policy, source)
	case "rotate_secret":
		if !policy.AutoHeal.AllowRotateSecret {
			return fmt.Errorf("heimdall guardian policy blocks rotate_secret actions")
		}
		return executeHeimdallSecretRotation(ctx, db, action)
	default:
		return fmt.Errorf("heimdall action type %q is not executable by the core loop", actionType)
	}
}

func executeHeimdallWorkflowDispatch(
	ctx context.Context,
	conn *amqp.Connection,
	db *sql.DB,
	action map[string]any,
	repositoryBindings map[string]heimdallRepositoryBinding,
	policy model.GuardianPolicyManifestSpec,
	source string,
) error {
	binding, err := resolveHeimdallActionBinding(action, repositoryBindings)
	if err != nil {
		return err
	}
	if !binding.Spec.Automation.AllowDispatchWorkflow {
		return fmt.Errorf("repository binding %s disables workflow dispatch", binding.Spec.Repository)
	}

	workflow := firstNonEmpty(anyString(action["workflow"]), binding.Spec.DeployWorkflow, defaultHeimdallDispatchWorkflow)
	ref := firstNonEmpty(anyString(action["ref"]), binding.Spec.DefaultBranch, defaultHeimdallDispatchBranch)
	repositoryName := firstNonEmpty(anyString(action["repository"]), binding.Spec.Repository)
	inputs := heimdallBuildWorkflowInputs(action, binding.Spec, source, nil)

	if !heimdallActionCooldownAllowed(action, binding.Spec, policy) {
		return fmt.Errorf("heimdall action for %s is still in cooldown", binding.Spec.Repository)
	}

	_, err = executeIntegrationRequest(ctx, conn, db, model.ExecuteIntegrationRequest{
		Integration: model.ManifestSelector{
			Namespace: "global",
			Name:      "github-caller",
		},
		Operation:  "dispatch_workflow",
		Capability: "dispatch_workflow",
		Input: map[string]any{
			"repository": repositoryName,
			"workflow":   workflow,
			"ref":        ref,
			"inputs":     inputs,
			"metadata": map[string]any{
				"source":              "core.heimdall.guardian_loop",
				"action":              action,
				"binding":             binding.Spec.Repository,
				"policy":              policy.Scope,
				"component_namespace": binding.Spec.ComponentNamespace,
				"dispatched":          time.Now().UTC().Format(time.RFC3339),
			},
		},
	}, 0)
	if err != nil {
		return err
	}

	markHeimdallActionCooldown(action, binding.Spec)
	return nil
}

func executeHeimdallContractAction(
	ctx context.Context,
	conn *amqp.Connection,
	db *sql.DB,
	action map[string]any,
	repositoryBindings map[string]heimdallRepositoryBinding,
	remediationContracts map[string]heimdallRemediationContract,
	policy model.GuardianPolicyManifestSpec,
	source string,
) error {
	contract, contractAction, err := resolveHeimdallContractAction(action, remediationContracts)
	if err != nil {
		return err
	}
	if !contractAction.AutoExecute {
		return fmt.Errorf("remediation contract %s/%s action %q is not marked auto_execute", contract.Manifest.Metadata.Namespace, contract.Manifest.Metadata.Name, contractAction.Name)
	}

	switch contractAction.Mode {
	case model.RemediationContractActionModeWorkflowDispatch:
		return executeHeimdallContractWorkflowDispatch(ctx, conn, db, action, repositoryBindings, contract, contractAction, policy, source)
	default:
		return fmt.Errorf("remediation contract action mode %q is unsupported", contractAction.Mode)
	}
}

func executeHeimdallContractWorkflowDispatch(
	ctx context.Context,
	conn *amqp.Connection,
	db *sql.DB,
	action map[string]any,
	repositoryBindings map[string]heimdallRepositoryBinding,
	contract heimdallRemediationContract,
	contractAction model.RemediationContractActionSpec,
	policy model.GuardianPolicyManifestSpec,
	source string,
) error {
	binding, err := heimdallWorkflowDispatchBinding(action, repositoryBindings, contract, contractAction)
	if err != nil {
		return err
	}
	if !heimdallActionCooldownAllowed(action, binding.Spec, policy) {
		return fmt.Errorf("heimdall action for %s is still in cooldown", binding.Spec.Repository)
	}

	repositoryName := firstNonEmpty(binding.Spec.Repository, contractAction.WorkflowDispatch.Repository)
	workflow := firstNonEmpty(contractAction.WorkflowDispatch.Workflow, binding.Spec.DeployWorkflow, defaultHeimdallDispatchWorkflow)
	ref := firstNonEmpty(contractAction.WorkflowDispatch.Ref, binding.Spec.DefaultBranch, defaultHeimdallDispatchBranch)
	inputs := heimdallBuildWorkflowInputs(action, binding.Spec, source, contractAction.WorkflowDispatch.Inputs)
	inputs["remediation_type"] = strings.ToLower(strings.TrimSpace(contractAction.Name))
	inputs["remediation_reason"] = firstNonEmpty(anyString(action["reason"]), anyString(action["description"]), source)
	inputs["remediation_payload"] = heimdallActionPayload(action)

	_, err = executeIntegrationRequest(ctx, conn, db, model.ExecuteIntegrationRequest{
		Integration: model.ManifestSelector{
			Namespace: "global",
			Name:      "github-caller",
		},
		Operation:  "dispatch_workflow",
		Capability: "dispatch_workflow",
		Input: map[string]any{
			"repository": repositoryName,
			"workflow":   workflow,
			"ref":        ref,
			"inputs":     inputs,
			"metadata": map[string]any{
				"source":              "core.heimdall.guardian_loop",
				"action":              action,
				"binding":             binding.Spec.Repository,
				"policy":              policy.Scope,
				"component_namespace": binding.Spec.ComponentNamespace,
				"dispatched":          time.Now().UTC().Format(time.RFC3339),
				"remediation_contract": map[string]any{
					"namespace": contract.Manifest.Metadata.Namespace,
					"name":      contract.Manifest.Metadata.Name,
					"action":    contractAction.Name,
					"mode":      contractAction.Mode,
				},
			},
		},
	}, 0)
	if err != nil {
		return err
	}

	markHeimdallActionCooldown(action, binding.Spec)
	return nil
}

func executeHeimdallSecretRotation(ctx context.Context, db *sql.DB, action map[string]any) error {
	namespace := firstNonEmpty(anyString(action["namespace"]), "global")
	name := strings.TrimSpace(anyString(action["name"]))
	if name == "" {
		return fmt.Errorf("rotate_secret action requires a secret name")
	}

	data := map[string]string{}
	switch typed := action["data"].(type) {
	case map[string]string:
		data = typed
	case map[string]any:
		for key, value := range typed {
			data[key] = anyString(value)
		}
	}
	if len(data) == 0 {
		return fmt.Errorf("rotate_secret action requires replacement data")
	}

	_, err := repository.RotateManagedSecret(ctx, db, model.RotateManagedSecretRequest{
		Namespace: namespace,
		Name:      name,
		Data:      data,
		Metadata: map[string]any{
			"source": "core.heimdall.guardian_loop",
		},
	})
	return err
}

func heimdallActionCooldownAllowed(action map[string]any, binding model.RepositoryBindingManifestSpec, policy model.GuardianPolicyManifestSpec) bool {
	cooldown := time.Duration(policy.AutoHeal.CooldownSeconds) * time.Second
	if cooldown <= 0 {
		return true
	}

	key := heimdallActionKey(action, binding)

	heimdallActionCooldownMu.Lock()
	defer heimdallActionCooldownMu.Unlock()

	lastRun, exists := heimdallActionCooldown[key]
	if !exists {
		return true
	}
	return time.Since(lastRun) >= cooldown
}

func markHeimdallActionCooldown(action map[string]any, binding model.RepositoryBindingManifestSpec) {
	heimdallActionCooldownMu.Lock()
	defer heimdallActionCooldownMu.Unlock()
	heimdallActionCooldown[heimdallActionKey(action, binding)] = time.Now()
}

func heimdallActionKey(action map[string]any, binding model.RepositoryBindingManifestSpec) string {
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(anyString(action["type"]))),
		strings.ToLower(strings.TrimSpace(binding.ComponentKind)),
		strings.ToLower(strings.TrimSpace(binding.ComponentNamespace)),
		strings.ToLower(strings.TrimSpace(binding.ComponentName)),
		strings.ToLower(strings.TrimSpace(binding.Repository)),
	}, "|")
}

func resolveHeimdallActionBinding(
	action map[string]any,
	repositoryBindings map[string]heimdallRepositoryBinding,
) (heimdallRepositoryBinding, error) {
	componentKind := firstNonEmpty(anyString(action["component_kind"]), anyString(action["kind"]))
	componentNamespace := firstNonEmpty(anyString(action["component_namespace"]), anyString(action["namespace"]), "global")
	componentName := firstNonEmpty(anyString(action["component_name"]), anyString(action["name"]))
	if componentKind != "" && componentName != "" {
		if binding, ok := repositoryBindings[heimdallComponentKey(componentKind, componentNamespace, componentName)]; ok {
			return binding, nil
		}
	}

	repositoryName := strings.TrimSpace(anyString(action["repository"]))
	if repositoryName == "" {
		repositoryName = strings.TrimSpace(anyString(action["repo"]))
	}
	if repositoryName != "" {
		for _, binding := range repositoryBindings {
			if strings.EqualFold(binding.Spec.Repository, repositoryName) {
				return binding, nil
			}
		}
	}

	return heimdallRepositoryBinding{}, fmt.Errorf("no repository binding matched Heimdall action")
}

func resolveHeimdallContractAction(
	action map[string]any,
	contracts map[string]heimdallRemediationContract,
) (heimdallRemediationContract, model.RemediationContractActionSpec, error) {
	componentKind := firstNonEmpty(anyString(action["component_kind"]), anyString(action["kind"]))
	componentNamespace := firstNonEmpty(anyString(action["component_namespace"]), anyString(action["namespace"]), "global")
	componentName := firstNonEmpty(anyString(action["component_name"]), anyString(action["name"]))
	if componentKind == "" || componentName == "" {
		return heimdallRemediationContract{}, model.RemediationContractActionSpec{}, fmt.Errorf("heimdall remediation action is missing component identity")
	}

	contract, ok := contracts[heimdallComponentKey(componentKind, componentNamespace, componentName)]
	if !ok {
		return heimdallRemediationContract{}, model.RemediationContractActionSpec{}, fmt.Errorf("no remediation contract matched Heimdall action for %s/%s", componentNamespace, componentName)
	}

	actionType := strings.ToLower(strings.TrimSpace(anyString(action["type"])))
	for _, candidate := range contract.Spec.Actions {
		if strings.EqualFold(candidate.Name, actionType) {
			return contract, candidate, nil
		}
	}

	return heimdallRemediationContract{}, model.RemediationContractActionSpec{}, fmt.Errorf("remediation contract %s/%s does not expose action %q", contract.Manifest.Metadata.Namespace, contract.Manifest.Metadata.Name, actionType)
}

func heimdallWorkflowDispatchBinding(
	action map[string]any,
	repositoryBindings map[string]heimdallRepositoryBinding,
	contract heimdallRemediationContract,
	contractAction model.RemediationContractActionSpec,
) (heimdallRepositoryBinding, error) {
	if binding, err := resolveHeimdallActionBinding(action, repositoryBindings); err == nil {
		if contractAction.WorkflowDispatch != nil {
			binding.Spec.Repository = firstNonEmpty(contractAction.WorkflowDispatch.Repository, binding.Spec.Repository)
			binding.Spec.DeployWorkflow = firstNonEmpty(contractAction.WorkflowDispatch.Workflow, binding.Spec.DeployWorkflow)
			binding.Spec.DefaultBranch = firstNonEmpty(contractAction.WorkflowDispatch.Ref, binding.Spec.DefaultBranch)
		}
		return binding, nil
	}

	if contractAction.WorkflowDispatch == nil {
		return heimdallRepositoryBinding{}, fmt.Errorf("remediation contract action does not define workflow dispatch settings")
	}

	repositoryName := strings.TrimSpace(contractAction.WorkflowDispatch.Repository)
	if repositoryName == "" {
		return heimdallRepositoryBinding{}, fmt.Errorf("remediation contract %s/%s requires either a repository binding or workflow_dispatch.repository", contract.Manifest.Metadata.Namespace, contract.Manifest.Metadata.Name)
	}

	return heimdallRepositoryBinding{
		Spec: model.RepositoryBindingManifestSpec{
			ComponentKind:      contract.Spec.ComponentKind,
			ComponentNamespace: contract.Spec.ComponentNamespace,
			ComponentName:      contract.Spec.ComponentName,
			Repository:         repositoryName,
			DefaultBranch:      contractAction.WorkflowDispatch.Ref,
			DeployWorkflow:     contractAction.WorkflowDispatch.Workflow,
			Automation: model.RepositoryBindingAutomationSpec{
				AllowDispatchWorkflow: true,
			},
			Metadata: cloneAuthorizationInput(contract.Spec.Metadata),
		},
	}, nil
}

func heimdallComponentKey(kind, namespace, name string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	namespace = strings.ToLower(strings.TrimSpace(namespace))
	if namespace == "" {
		namespace = "global"
	}
	name = strings.ToLower(strings.TrimSpace(name))
	return kind + "|" + namespace + "|" + name
}

func heimdallNumericDetail(details map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		value, ok := details[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return typed, true
		case int:
			return float64(typed), true
		case int64:
			return float64(typed), true
		}
	}
	return 0, false
}

func heimdallRemediationActions(output any) []map[string]any {
	return heimdallMapSliceField(output, "actions")
}

func heimdallOutputIncidentSeverity(output any) string {
	object, ok := output.(map[string]any)
	if !ok {
		return ""
	}
	incident, ok := object["incident"].(map[string]any)
	if !ok {
		return ""
	}
	return anyString(incident["severity"])
}

func heimdallSeverityMeetsThreshold(severity string, threshold string) bool {
	order := map[string]int{
		"low":      1,
		"medium":   2,
		"high":     3,
		"critical": 4,
	}
	current := order[strings.ToLower(strings.TrimSpace(severity))]
	required := order[strings.ToLower(strings.TrimSpace(threshold))]
	if current == 0 || required == 0 {
		return false
	}
	return current >= required
}

func heimdallCostActions(output any, policy model.GuardianPolicyManifestSpec) []map[string]any {
	opportunities := heimdallMapSliceField(output, "opportunities")
	actions := make([]map[string]any, 0, len(opportunities))
	for _, opportunity := range opportunities {
		savings, _ := heimdallNumericDetail(opportunity, "estimated_monthly_save_usd", "estimated_monthly_savings_usd")
		if savings < policy.CostOptimization.MinEstimatedMonthlySavingsUSD {
			continue
		}
		action, ok := opportunity["action"].(map[string]any)
		if !ok || len(action) == 0 {
			continue
		}
		actions = append(actions, cloneAuthorizationInput(action))
	}
	return actions
}

func heimdallRuntimeIndicatesOOM(state *model.IntegrationRuntimeState) bool {
	if state == nil {
		return false
	}
	if heimdallBoolDetail(state.Details, "oom_killed", "oom", "memory_pressure") {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(state.Message))
	return strings.Contains(message, "oom") || strings.Contains(message, "out of memory")
}

func heimdallBoolDetail(details map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := details[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			if typed {
				return true
			}
		case string:
			normalized := strings.ToLower(strings.TrimSpace(typed))
			if normalized == "true" || normalized == "yes" || normalized == "1" || normalized == "oomkilled" {
				return true
			}
		}
	}
	return false
}

func heimdallBuildWorkflowInputs(
	action map[string]any,
	binding model.RepositoryBindingManifestSpec,
	source string,
	extraInputs map[string]any,
) map[string]any {
	inputs := map[string]any{}
	if workflowInputs := heimdallWorkflowInputsFromAction(action); len(workflowInputs) > 0 {
		inputs = mergeStringAnyMaps(inputs, workflowInputs)
	}
	if len(extraInputs) > 0 {
		inputs = mergeStringAnyMaps(inputs, cloneAuthorizationInput(extraInputs))
	}

	inputs["component_kind"] = firstNonEmpty(anyString(action["component_kind"]), binding.ComponentKind)
	inputs["component_name"] = firstNonEmpty(anyString(action["component_name"]), binding.ComponentName)
	inputs["component_namespace"] = firstNonEmpty(anyString(action["component_namespace"]), binding.ComponentNamespace, "global")
	inputs["commit_sha"] = ""
	inputs["actor"] = defaultHeimdallDispatchActor
	inputs["event_name"] = source
	inputs["environment"] = firstNonEmpty(anyString(binding.Metadata["environment"]), defaultHeimdallDispatchEnvironment)
	inputs["commit_message"] = firstNonEmpty(anyString(action["reason"]), source)
	inputs["source_run_url"] = ""
	inputs["source_run_id"] = "heimdall-guardian-loop"

	return inputs
}

func heimdallWorkflowInputsFromAction(action map[string]any) map[string]any {
	workflow, ok := action["workflow"].(map[string]any)
	if !ok {
		return nil
	}
	switch typed := workflow["inputs"].(type) {
	case map[string]any:
		return cloneAuthorizationInput(typed)
	case map[string]string:
		result := make(map[string]any, len(typed))
		for key, value := range typed {
			result[key] = value
		}
		return result
	default:
		return nil
	}
}

func heimdallActionPayload(action map[string]any) string {
	payload := cloneAuthorizationInput(action)
	data, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func heimdallMapSliceField(output any, field string) []map[string]any {
	object, ok := output.(map[string]any)
	if !ok {
		return nil
	}
	rawItems, ok := object[field].([]any)
	if !ok {
		return nil
	}
	items := make([]map[string]any, 0, len(rawItems))
	for _, rawItem := range rawItems {
		if item, ok := rawItem.(map[string]any); ok {
			items = append(items, cloneAuthorizationInput(item))
		}
	}
	return items
}

func anyString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}
