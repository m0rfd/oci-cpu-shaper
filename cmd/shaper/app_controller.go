package main

import (
	"context"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/adapt"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

type controllerRuntime struct {
	cfg             runtimeconfig.Config
	opts            options
	logger          *zap.Logger
	imdsClient      imds.Client
	metricsExporter *metricshttp.Exporter
	metricsShutdown metricsShutdownFunc
	metricsCancel   context.CancelFunc
	controller      adapt.Controller
	pool            poolStarter
}

func (c controllerRuntime) cleanup(ctx context.Context) {
	if c.metricsShutdown == nil {
		return
	}

	if c.metricsCancel != nil {
		c.metricsCancel()
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(
		context.WithoutCancel(ctx),
		metricsShutdownTimeout,
	)
	defer cancelShutdown()

	c.metricsShutdown(shutdownCtx)
}

func (c controllerRuntime) start(ctx context.Context) int {
	if c.pool != nil {
		c.pool.SetWorkerStartErrorHandler(func(err error) {
			if err == nil {
				return
			}

			c.logger.Warn("worker failed to enter sched_idle", zap.Error(err))
		})

		c.logger.Info(
			"starting worker pool",
			zap.Int("workers", c.pool.Workers()),
			zap.Duration("quantum", c.pool.Quantum()),
		)

		c.pool.Start(ctx)
	}

	logIMDSMetadata(
		ctx,
		c.logger,
		c.imdsClient,
		c.controller,
		c.cfg.OCI.InstanceID,
		c.cfg.OCI.CompartmentID,
		c.cfg.OCI.Region,
		c.cfg.OCI.Offline,
	)

	c.logger.Info(
		"starting controller run",
		zap.String("mode", c.controller.Mode()),
		zap.String("controllerState", c.controller.State().String()),
	)

	return handleControllerRunResult(c.logger, c.controller.Run(ctx))
}

//nolint:funlen // wiring stage sequences metadata, metrics, and controller bootstrap.
func (a app) prepareController(
	ctx context.Context,
	boot bootstrapResult,
) (controllerRuntime, int, bool) {
	imdsClient := boot.deps.newIMDS()

	metricsExporter := buildMetricsExporter(boot.deps)
	cgroupInfo := detectAndReportCgroup(boot.deps, boot.logger, metricsExporter)
	metricsRecorder := newRecorderLogger(boot.logger, metricsExporter)

	cfg, metadata, metadataErr := prepareRunMetadata(ctx, boot.cfg, imdsClient, boot.opts.mode)
	if metadataErr != nil {
		boot.logger.Error("failed to resolve oci metadata", zap.Error(metadataErr))

		var empty controllerRuntime

		return empty, exitCodeRuntimeError, false
	}

	logMetadataResolution(boot.logger, boot.opts.mode, metadata, cfg.OCI.Offline)

	controller, pool, buildErr := boot.deps.newController(
		ctx,
		boot.opts.mode,
		cfg,
		imdsClient,
		metricsRecorder,
	)
	if buildErr != nil {
		code := exitCodeForConfigError(buildErr)

		boot.logger.Error("failed to build controller", zap.Error(buildErr))

		var empty controllerRuntime

		return empty, code, false
	}

	logControllerInitialization(boot.logger, cfg, controller, metricsExporter)

	metricsHandler := configureMetrics(boot.logger, metricsExporter, pool, controller, cgroupInfo)

	metricsShutdown, metricsCancel, metricsErr := startMetricsEndpoint(
		ctx,
		boot.deps,
		boot.logger,
		cfg.HTTP.Bind,
		metricsHandler,
	)
	if metricsErr != nil {
		boot.logger.Error("failed to start metrics server", zap.Error(metricsErr))

		var empty controllerRuntime

		return empty, exitCodeRuntimeError, false
	}

	return controllerRuntime{
		cfg:             cfg,
		opts:            boot.opts,
		logger:          boot.logger,
		imdsClient:      imdsClient,
		metricsExporter: metricsExporter,
		metricsShutdown: metricsShutdown,
		metricsCancel:   metricsCancel,
		controller:      controller,
		pool:            pool,
	}, exitCodeSuccess, true
}
