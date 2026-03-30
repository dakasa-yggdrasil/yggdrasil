package message

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	manifestengine "github.com/dakasa-co/yggdrasil-core/manifest"
	"github.com/dakasa-co/yggdrasil-core/model"
	"github.com/dakasa-co/yggdrasil-core/repository"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const defaultIntegrationRuntimeMonitorInterval = 60 * time.Second

// StartIntegrationRuntimeMonitor runs a background sweep that refreshes integration type handshake state.
func StartIntegrationRuntimeMonitor(
	conn *amqp.Connection,
	db *sql.DB,
	logger *zap.Logger,
	interval time.Duration,
) context.CancelFunc {
	if interval <= 0 {
		interval = defaultIntegrationRuntimeMonitorInterval
	}

	ctx, cancel := context.WithCancel(context.Background())
	go runIntegrationRuntimeMonitor(ctx, conn, db, logger, interval)
	return cancel
}

func runIntegrationRuntimeMonitor(
	ctx context.Context,
	conn *amqp.Connection,
	db *sql.DB,
	logger *zap.Logger,
	interval time.Duration,
) {
	runIntegrationRuntimeMonitorSweep(ctx, conn, db, logger)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runIntegrationRuntimeMonitorSweep(ctx, conn, db, logger)
		}
	}
}

func runIntegrationRuntimeMonitorSweep(
	ctx context.Context,
	conn *amqp.Connection,
	db *sql.DB,
	logger *zap.Logger,
) {
	manifests, err := repository.ListManifests(ctx, db, model.ListManifestFilters{
		Kind:       "integration_type",
		ActiveOnly: true,
	})
	if err != nil {
		if logger != nil {
			logger.Warn("integration runtime monitor failed to list integration types", zap.Error(err))
		}
		return
	}

	for _, manifestRecord := range manifests {
		spec, err := manifestengine.ParseIntegrationTypeSpec(manifestRecord.Spec)
		if err != nil {
			recordErr := fmt.Errorf(
				"parse integration_type %s/%s for runtime monitor: %w",
				manifestRecord.Metadata.Namespace,
				manifestRecord.Metadata.Name,
				err,
			)
			_ = failIntegrationDescribeHandshake(
				ctx,
				db,
				manifestRecord,
				model.IntegrationRuntimeStatusContractMismatch,
				recordErr,
				map[string]any{
					"integration_type": manifestReferenceFromRecord(manifestRecord),
					"source":           "integration_runtime_monitor",
				},
			)
			if logger != nil {
				logger.Warn("integration runtime monitor found invalid integration_type manifest", zap.Error(recordErr))
			}
			continue
		}

		if err := verifyResolvedIntegrationType(ctx, conn, db, manifestRecord, spec); err != nil && logger != nil {
			logger.Warn(
				"integration runtime monitor handshake failed",
				zap.String("namespace", manifestRecord.Metadata.Namespace),
				zap.String("name", manifestRecord.Metadata.Name),
				zap.Error(err),
			)
		}
	}
}
