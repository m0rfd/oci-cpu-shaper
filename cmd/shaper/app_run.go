package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"go.uber.org/zap"
)

// run orchestrates CLI initialization before handing execution to the controller.
//
//nolint:funlen,cyclop // CLI wiring composes setup steps before controller execution
func (a app) run(
	ctx context.Context,
	args []string,
	stderr io.Writer,
) int {
	deps := a.deps

	opts, err := parseArgs(args)
	if err != nil {
		return writeError(stderr, err, exitCodeParseError)
	}

	if opts.showVersion {
		info := deps.currentBuildInfo()

		writer := deps.versionWriter
		if writer == nil {
			writer = os.Stdout
		}

		_, _ = fmt.Fprintf(writer, "%+v\n", info)

		return exitCodeSuccess
	}

	cfg, exitCode, configLoaded := loadRuntimeConfigOrExit(deps, opts.configPath, stderr)
	if !configLoaded {
		return exitCode
	}

	logger, exitCode, loggerReady := buildLoggerOrExit(deps, opts.logLevel, stderr)
	if !loggerReady {
		return exitCode
	}

	defer func() {
		_ = logger.Sync()
	}()

	ctx, cancel := applyShutdownTimer(ctx, opts.shutdownAfter)
	if cancel != nil {
		defer cancel()
	}

	info := deps.currentBuildInfo()
	logStartup(logger, info, opts)
	logRuntimeConfig(logger, cfg)

	imdsClient := deps.newIMDS()

	metricsExporter := buildMetricsExporter(deps)
	cgroupInfo := detectAndReportCgroup(deps, logger, metricsExporter)
	metricsRecorder := newRecorderLogger(logger, metricsExporter)

	cfg, metadata, metadataErr := prepareRunMetadata(ctx, cfg, imdsClient, opts.mode)
	if metadataErr != nil {
		logger.Error("failed to resolve oci metadata", zap.Error(metadataErr))

		return exitCodeRuntimeError
	}

	logMetadataResolution(logger, opts.mode, metadata, cfg.OCI.Offline)

	controller, pool, buildErr := deps.newController(
		ctx,
		opts.mode,
		cfg,
		imdsClient,
		metricsRecorder,
	)
	if buildErr != nil {
		code := exitCodeForConfigError(buildErr)

		logger.Error("failed to build controller", zap.Error(buildErr))

		return code
	}

	logControllerInitialization(logger, cfg, controller, metricsExporter)

	metricsHandler := configureMetrics(logger, metricsExporter, pool, controller, cgroupInfo)

	metricsShutdown, metricsCancel, metricsErr := startMetricsEndpoint(
		ctx,
		deps,
		logger,
		cfg.HTTP.Bind,
		metricsHandler,
	)
	if metricsErr != nil {
		logger.Error("failed to start metrics server", zap.Error(metricsErr))

		return exitCodeRuntimeError
	}

	if metricsShutdown != nil {
		defer func() {
			if metricsCancel != nil {
				metricsCancel()
			}

			shutdownCtx, cancelShutdown := context.WithTimeout(
				context.WithoutCancel(ctx),
				metricsShutdownTimeout,
			)
			defer cancelShutdown()

			metricsShutdown(shutdownCtx)
		}()
	}

	if pool != nil {
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

	logIMDSMetadata(
		ctx,
		logger,
		imdsClient,
		controller,
		cfg.OCI.InstanceID,
		cfg.OCI.CompartmentID,
		cfg.OCI.Region,
		cfg.OCI.Offline,
	)

	logger.Info(
		"starting controller run",
		zap.String("mode", controller.Mode()),
		zap.String("controllerState", controller.State().String()),
	)

	return handleControllerRunResult(logger, controller.Run(ctx))
}

func run(ctx context.Context, args []string, deps runDeps, stderr io.Writer) int {
	return newApp(deps).run(ctx, args, stderr)
}
