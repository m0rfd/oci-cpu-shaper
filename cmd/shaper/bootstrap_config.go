package main

import (
	"context"
	"io"

	"go.uber.org/zap"
	"oci-cpu-shaper/internal/buildinfo"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

type configBootstrap struct {
	cfg    runtimeconfig.Config
	logger *zap.Logger
	cancel context.CancelFunc
	info   buildinfo.Info
}

func stageConfig(
	ctx context.Context,
	deps runDeps,
	opts options,
	stderr io.Writer,
) (context.Context, configBootstrap, int, bool) {
	cfg, exitCode, configLoaded := loadRuntimeConfigOrExit(deps, opts.configPath, stderr)
	if !configLoaded {
		var empty configBootstrap

		return ctx, empty, exitCode, false
	}

	logger, exitCode, loggerReady := buildLoggerOrExit(deps, opts.logLevel, stderr)
	if !loggerReady {
		var empty configBootstrap

		return ctx, empty, exitCode, false
	}

	ctx, cancel := applyShutdownTimer(ctx, opts.shutdownAfter)

	info := deps.currentBuildInfo()
	logStartup(logger, info, opts)
	logRuntimeConfig(logger, cfg)

	return ctx, configBootstrap{
		cfg:    cfg,
		logger: logger,
		cancel: cancel,
		info:   info,
	}, exitCodeSuccess, true
}
