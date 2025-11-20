package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
	"oci-cpu-shaper/internal/buildinfo"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/cgroup"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

const (
	defaultConfigPath = "/etc/oci-cpu-shaper/config.yaml"
	defaultLogLevel   = "info"
	modeDryRun        = "dry-run"
	modeEnforce       = "enforce"
	modeNoop          = "noop"

	imdsEndpointEnv = "OCI_CPU_SHAPER_IMDS_ENDPOINT"

	offlineInstanceFallback = "offline-instance"

	exitCodeSuccess      = 0
	exitCodeRuntimeError = 1
	exitCodeParseError   = 2
)

type runDeps struct {
	newLogger     func(level string) (*zap.Logger, error)
	newIMDS       func() imds.Client
	newController func(
		ctx context.Context,
		mode string,
		cfg runtimeconfig.Config,
		imdsClient imds.Client,
		recorder adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error)
	currentBuildInfo   func() buildinfo.Info
	loadConfig         func(path string) (runtimeconfig.Config, error)
	newMetricsExporter func() *metricshttp.Exporter
	startMetricsServer func(
		ctx context.Context,
		logger *zap.Logger,
		addr string,
		handler http.Handler,
	) (metricsShutdownFunc, error)
	versionWriter io.Writer
	detectCgroup  func() (*cgroup.CPU, error)
}

type poolStarter interface {
	Start(ctx context.Context)
	Workers() int
	Quantum() time.Duration
	SetWorkerStartErrorHandler(handler func(err error))
}

func writeError(dst io.Writer, err error, code int) int {
	if err == nil {
		return code
	}

	_, ferr := fmt.Fprintf(dst, "%v\n", err)
	if ferr != nil {
		return code
	}

	return code
}

func loadRuntimeConfigOrExit(
	deps runDeps,
	path string,
	stderr io.Writer,
) (runtimeconfig.Config, int, bool) {
	cfg, loadErr := deps.loadConfig(path)
	if loadErr != nil {
		code := exitCodeForConfigError(loadErr)

		exitCode := writeError(
			stderr,
			fmt.Errorf("failed to load configuration: %w", loadErr),
			code,
		)

		var empty runtimeconfig.Config

		return empty, exitCode, false
	}

	return cfg, exitCodeSuccess, true
}

func buildLoggerOrExit(
	deps runDeps,
	level string,
	stderr io.Writer,
) (*zap.Logger, int, bool) {
	logger, loggerErr := deps.newLogger(level)
	if loggerErr != nil {
		exitCode := writeError(
			stderr,
			fmt.Errorf("failed to configure logger: %w", loggerErr),
			exitCodeRuntimeError,
		)

		return nil, exitCode, false
	}

	return logger, exitCodeSuccess, true
}

//nolint:ireturn // factory intentionally returns controller interface for wiring flexibility.
func defaultControllerFactory(
	ctx context.Context,
	mode string,
	cfg runtimeconfig.Config,
	imdsClient imds.Client,
	recorder adapt.MetricsRecorder,
) (adapt.Controller, poolStarter, error) {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" {
		trimmed = modeDryRun
	}

	if trimmed == modeNoop {
		if recorder != nil {
			recorder.SetMode(trimmed)
			recorder.SetState(adapt.StateNormal.String())
			recorder.SetTarget(0)
		}

		return adapt.NewNoopController(trimmed), nil, nil
	}

	if imdsClient == nil {
		return nil, nil, errControllerIMDSRequired
	}

	return buildAdaptiveController(ctx, trimmed, cfg, imdsClient, recorder)
}

func applyShutdownTimer(
	ctx context.Context,
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, nil
	}

	newCtx, cancel := context.WithTimeout(ctx, timeout)

	return newCtx, cancel
}
