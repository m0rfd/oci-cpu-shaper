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
	return startController(ctx, c)
}

func (a app) prepareController(
	ctx context.Context,
	boot bootstrapResult,
) (controllerRuntime, int, bool) {
	metadataStage, exitCode, ready := stageMetadata(ctx, boot)
	if !ready {
		var empty controllerRuntime

		return empty, exitCode, false
	}

	metricsExporter := buildMetricsExporter(boot.deps)
	metricsRecorder := newRecorderLogger(boot.logger, metricsExporter)

	controller, pool, buildErr := boot.deps.newController(
		ctx,
		boot.opts.mode,
		metadataStage.cfg,
		metadataStage.imdsClient,
		metricsRecorder,
	)
	if buildErr != nil {
		code := exitCodeForConfigError(buildErr)

		boot.logger.Error("failed to build controller", zap.Error(buildErr))

		var empty controllerRuntime

		return empty, code, false
	}

	metricsStage, metricsErr := stageMetrics(
		ctx,
		boot.deps,
		boot.logger,
		metadataStage.cfg,
		pool,
		controller,
		metricsExporter,
	)
	if metricsErr != nil {
		boot.logger.Error("failed to start metrics server", zap.Error(metricsErr))

		var empty controllerRuntime

		return empty, exitCodeRuntimeError, false
	}

	logControllerInitialization(boot.logger, metadataStage.cfg, controller, metricsStage.exporter)

	return controllerRuntime{
		cfg:             metadataStage.cfg,
		opts:            boot.opts,
		logger:          boot.logger,
		imdsClient:      metadataStage.imdsClient,
		metricsExporter: metricsStage.exporter,
		metricsShutdown: metricsStage.shutdown,
		metricsCancel:   metricsStage.cancel,
		controller:      controller,
		pool:            pool,
	}, exitCodeSuccess, true
}
