package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"go.uber.org/zap"
	"oci-cpu-shaper/internal/buildinfo"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

type bootstrapResult struct {
	opts   options
	cfg    runtimeconfig.Config
	logger *zap.Logger
	cancel context.CancelFunc
	info   buildinfo.Info
	stderr io.Writer
	deps   runDeps
}

func (r bootstrapResult) cleanup() {
	if r.cancel != nil {
		r.cancel()
	}

	if r.logger != nil {
		_ = r.logger.Sync()
	}
}

func (a app) parseOptions(args []string, stderr io.Writer) (options, int, bool) {
	opts, err := parseArgs(args)
	if err != nil {
		var empty options

		return empty, writeError(stderr, err, exitCodeParseError), false
	}

	if opts.showVersion {
		info := a.deps.currentBuildInfo()

		writer := a.deps.versionWriter
		if writer == nil {
			writer = os.Stdout
		}

		_, _ = fmt.Fprintf(writer, "%+v\n", info)

		var empty options

		return empty, exitCodeSuccess, false
	}

	return opts, exitCodeSuccess, true
}

func (a app) bootstrap(
	ctx context.Context,
	args []string,
	stderr io.Writer,
) (context.Context, bootstrapResult, int, bool) {
	opts, exitCode, proceed := a.parseOptions(args, stderr)
	if !proceed {
		var empty bootstrapResult

		return ctx, empty, exitCode, false
	}

	cfg, exitCode, configLoaded := loadRuntimeConfigOrExit(a.deps, opts.configPath, stderr)
	if !configLoaded {
		var empty bootstrapResult

		return ctx, empty, exitCode, false
	}

	logger, exitCode, loggerReady := buildLoggerOrExit(a.deps, opts.logLevel, stderr)
	if !loggerReady {
		var empty bootstrapResult

		return ctx, empty, exitCode, false
	}

	ctx, cancel := applyShutdownTimer(ctx, opts.shutdownAfter)

	info := a.deps.currentBuildInfo()
	logStartup(logger, info, opts)
	logRuntimeConfig(logger, cfg)

	return ctx, bootstrapResult{
		opts:   opts,
		cfg:    cfg,
		logger: logger,
		cancel: cancel,
		info:   info,
		stderr: stderr,
		deps:   a.deps,
	}, exitCodeSuccess, true
}
