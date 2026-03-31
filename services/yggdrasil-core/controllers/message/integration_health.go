package message

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	manifestengine "github.com/dakasa-co/yggdrasil-core/manifest"
	"github.com/dakasa-co/yggdrasil-core/model"
	"github.com/dakasa-co/yggdrasil-core/repository"
)

const integrationRuntimeFastFailWindow = 2 * defaultIntegrationRuntimeMonitorInterval

type integrationHealthGateError struct {
	Health model.IntegrationInstanceHealth
	Reason string
}

func (e *integrationHealthGateError) Error() string {
	if e == nil {
		return ""
	}

	return fmt.Sprintf(
		"integration instance %s/%s is unavailable for operations: status=%s reason=%s",
		e.Health.IntegrationInstance.Namespace,
		e.Health.IntegrationInstance.Name,
		e.Health.Status,
		strings.TrimSpace(e.Reason),
	)
}

func getIntegrationInstanceHealth(
	ctx context.Context,
	db *sql.DB,
	selector model.ManifestSelector,
	checkKind string,
) (model.IntegrationInstanceHealth, error) {
	instanceManifest, err := resolveManifestForKind(
		ctx,
		db,
		"integration_instance",
		selector.ManifestID,
		selector.Namespace,
		selector.Name,
		selector.Version,
	)
	if err != nil {
		return model.IntegrationInstanceHealth{}, err
	}

	return buildIntegrationInstanceHealth(ctx, db, instanceManifest, checkKind)
}

func listIntegrationInstanceHealth(
	ctx context.Context,
	db *sql.DB,
	req model.ListIntegrationInstanceHealthRequest,
) ([]model.IntegrationInstanceHealth, error) {
	checkKind := normalizeIntegrationInstanceHealthCheckKind(req.CheckKind)

	manifests, err := repository.ListManifests(ctx, db, model.ListManifestFilters{
		Kind:       "integration_instance",
		Namespace:  req.Namespace,
		Name:       req.Name,
		ActiveOnly: true,
	})
	if err != nil {
		return nil, err
	}

	filterStatus := strings.ToLower(strings.TrimSpace(req.Status))
	items := make([]model.IntegrationInstanceHealth, 0, len(manifests))
	for _, manifestRecord := range manifests {
		health, err := buildIntegrationInstanceHealth(ctx, db, manifestRecord, checkKind)
		if err != nil {
			return nil, err
		}
		if filterStatus != "" && health.Status != filterStatus {
			continue
		}
		items = append(items, health)
	}

	return items, nil
}

func buildIntegrationInstanceHealth(
	ctx context.Context,
	db *sql.DB,
	instanceManifest model.Manifest,
	checkKind string,
) (model.IntegrationInstanceHealth, error) {
	checkKind = normalizeIntegrationInstanceHealthCheckKind(checkKind)

	instanceSpec, err := manifestengine.ParseIntegrationInstanceSpec(instanceManifest.Spec)
	if err != nil {
		return model.IntegrationInstanceHealth{}, err
	}

	typeManifest, err := resolveManifestForKind(
		ctx,
		db,
		"integration_type",
		instanceSpec.TypeRef.ManifestID,
		instanceSpec.TypeRef.Namespace,
		instanceSpec.TypeRef.Name,
		instanceSpec.TypeRef.Version,
	)
	if err != nil {
		return model.IntegrationInstanceHealth{}, err
	}

	declaredStatus := normalizeIntegrationInstanceDeclaredStatus(instanceSpec.Status)
	health := model.IntegrationInstanceHealth{
		IntegrationInstance: manifestReferenceFromRecord(instanceManifest),
		IntegrationType:     manifestReferenceFromRecord(typeManifest),
		DeclaredStatus:      declaredStatus,
		CheckKind:           checkKind,
		Status:              integrationInstanceOverallStatus(declaredStatus, nil),
	}

	if declaredStatus != "active" {
		return health, nil
	}

	runtimeState, err := repository.GetIntegrationRuntimeState(ctx, db, model.ManifestSelector{
		ManifestID: instanceManifest.ID.String(),
		Namespace:  instanceManifest.Metadata.Namespace,
		Name:       instanceManifest.Metadata.Name,
		Version:    &instanceManifest.Version,
	}, checkKind)
	if err != nil {
		if errors.Is(err, repository.ErrIntegrationRuntimeStateNotFound) {
			health.Status = integrationInstanceOverallStatus(declaredStatus, nil)
			return health, nil
		}
		return model.IntegrationInstanceHealth{}, err
	}

	health.Status = integrationInstanceOverallStatus(declaredStatus, &runtimeState)
	health.RuntimeState = &runtimeState
	return health, nil
}

func preflightIntegrationInstanceHealth(
	ctx context.Context,
	db *sql.DB,
	instanceManifest model.Manifest,
	instanceSpec model.IntegrationInstanceManifestSpec,
	typeManifest model.Manifest,
	checkKind string,
) error {
	checkKind = normalizeIntegrationInstanceHealthCheckKind(checkKind)
	declaredStatus := normalizeIntegrationInstanceDeclaredStatus(instanceSpec.Status)
	health := model.IntegrationInstanceHealth{
		IntegrationInstance: manifestReferenceFromRecord(instanceManifest),
		IntegrationType:     manifestReferenceFromRecord(typeManifest),
		DeclaredStatus:      declaredStatus,
		CheckKind:           checkKind,
		Status:              integrationInstanceOverallStatus(declaredStatus, nil),
	}

	if declaredStatus != "active" {
		return &integrationHealthGateError{
			Health: health,
			Reason: "integration instance is not active",
		}
	}

	runtimeState, err := repository.GetIntegrationRuntimeState(ctx, db, model.ManifestSelector{
		ManifestID: instanceManifest.ID.String(),
		Namespace:  instanceManifest.Metadata.Namespace,
		Name:       instanceManifest.Metadata.Name,
		Version:    &instanceManifest.Version,
	}, checkKind)
	if err != nil {
		if errors.Is(err, repository.ErrIntegrationRuntimeStateNotFound) {
			return nil
		}
		return err
	}

	health.RuntimeState = &runtimeState
	health.Status = integrationInstanceOverallStatus(declaredStatus, &runtimeState)
	if shouldFastFailIntegrationRuntimeState(runtimeState, time.Now().UTC()) {
		return &integrationHealthGateError{
			Health: health,
			Reason: "recent runtime state is unhealthy",
		}
	}

	return nil
}

func normalizeIntegrationInstanceHealthCheckKind(checkKind string) string {
	checkKind = strings.ToLower(strings.TrimSpace(checkKind))
	if checkKind == "" {
		return model.IntegrationRuntimeCheckKindDescribeHandshake
	}
	return checkKind
}

func normalizeIntegrationInstanceDeclaredStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return "active"
	}
	return status
}

func integrationInstanceOverallStatus(declaredStatus string, runtimeState *model.IntegrationRuntimeState) string {
	declaredStatus = normalizeIntegrationInstanceDeclaredStatus(declaredStatus)
	if declaredStatus != "active" {
		return declaredStatus
	}
	if runtimeState == nil {
		return model.IntegrationInstanceHealthStatusUnknown
	}
	return strings.ToLower(strings.TrimSpace(runtimeState.Status))
}

func shouldFastFailIntegrationRuntimeState(state model.IntegrationRuntimeState, now time.Time) bool {
	status := strings.ToLower(strings.TrimSpace(state.Status))
	switch status {
	case model.IntegrationRuntimeStatusContractMismatch,
		model.IntegrationRuntimeStatusInvalidResponse,
		model.IntegrationRuntimeStatusUnreachable:
	default:
		return false
	}

	if state.LastCheckedAt.IsZero() {
		return true
	}

	return now.Sub(state.LastCheckedAt.UTC()) <= integrationRuntimeFastFailWindow
}

func integrationInstanceHealthErrorCode(err error) string {
	if isIntegrationHealthGateError(err) {
		return "integration_unhealthy"
	}
	if errors.Is(err, repository.ErrManifestNotFound) {
		return "not_found"
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(message, "integration") || strings.Contains(message, "manifest") || strings.Contains(message, "parse") {
		return "bad_request"
	}
	return "internal_error"
}

func isIntegrationHealthGateError(err error) bool {
	var target *integrationHealthGateError
	return errors.As(err, &target)
}

func integrationAwareErrorCode(err error, fallback string) string {
	if isIntegrationHealthGateError(err) {
		return "integration_unhealthy"
	}

	code := manifestLookupErrorCode(err)
	if code != "internal_error" {
		return code
	}

	return fallback
}
