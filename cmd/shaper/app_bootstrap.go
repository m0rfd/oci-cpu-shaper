package main

import (
	"context"
	"io"

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

func (a app) bootstrap(
	ctx context.Context,
	args []string,
	stderr io.Writer,
) (context.Context, bootstrapResult, int, bool) {
	opts, exitCode, proceed := parseOptionsOrPrintVersion(a.deps, args, stderr)
	if !proceed {
		var empty bootstrapResult

		return ctx, empty, exitCode, false
	}

	ctx, configStage, exitCode, ready := stageConfig(ctx, a.deps, opts, stderr)
	if !ready {
		var empty bootstrapResult

		return ctx, empty, exitCode, false
	}

	return ctx, bootstrapResult{
		opts:   opts,
		cfg:    configStage.cfg,
		logger: configStage.logger,
		cancel: configStage.cancel,
		info:   configStage.info,
		stderr: stderr,
		deps:   a.deps,
	}, exitCodeSuccess, true
}
