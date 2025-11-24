package main

import (
	"context"

	"go.uber.org/zap"
)

func startWorkerPool(ctx context.Context, logger *zap.Logger, pool poolStarter) {
	if pool == nil {
		return
	}

	pool.SetWorkerStartErrorHandler(func(err error) {
		if err == nil {
			return
		}

		logger.Warn("worker failed to enter sched_idle", zap.Error(err))
	})

	logger.Info(
		"starting worker pool",
		zap.Int("workers", pool.Workers()),
		zap.Duration("quantum", pool.Quantum()),
	)

	pool.Start(ctx)
}

func startController(ctx context.Context, runtime controllerRuntime) int {
	startWorkerPool(ctx, runtime.logger, runtime.pool)

	logIMDSMetadata(
		ctx,
		runtime.logger,
		runtime.imdsClient,
		runtime.controller,
		runtime.cfg.OCI.InstanceID,
		runtime.cfg.OCI.CompartmentID,
		runtime.cfg.OCI.Region,
		runtime.cfg.OCI.Offline,
	)

	runtime.logger.Info(
		"starting controller run",
		zap.String("mode", runtime.controller.Mode()),
		zap.String("controllerState", runtime.controller.State().String()),
	)

	return handleControllerRunResult(runtime.logger, runtime.controller.Run(ctx))
}
