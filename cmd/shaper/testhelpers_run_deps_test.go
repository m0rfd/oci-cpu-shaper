package main

import (
	"context"
	"io"
	"net/http"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/cgroup"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

// MetricsShutdownFunc mirrors metricsShutdownFunc for cross-package test usage.
type MetricsShutdownFunc = metricsShutdownFunc

// PoolStarter exposes poolStarter to integration tests without leaking it to production code.
type PoolStarter = poolStarter

// IntegrationRunner wires runDeps overrides for integration tests.
type IntegrationRunner struct {
	deps runDeps
}

// NewIntegrationRunner clones the default run dependencies for integration scenarios.
func NewIntegrationRunner() *IntegrationRunner {
	return &IntegrationRunner{deps: defaultRunDeps()}
}

// WithLoggerFactory overrides the logger constructor.
func (r *IntegrationRunner) WithLoggerFactory(factory func(string) (*zap.Logger, error)) {
	r.deps.newLogger = factory
}

// WithConfigLoader overrides the runtime configuration loader.
func (r *IntegrationRunner) WithConfigLoader(loader func(string) (runtimeconfig.Config, error)) {
	r.deps.loadConfig = loader
}

// WithStartMetricsServer swaps the metrics server starter.
func (r *IntegrationRunner) WithStartMetricsServer(
	starter func(context.Context, *zap.Logger, string, http.Handler) (func(context.Context), error),
) {
	r.deps.startMetricsServer = func(
		ctx context.Context,
		logger *zap.Logger,
		addr string,
		handler http.Handler,
	) (metricsShutdownFunc, error) {
		shutdown, err := starter(ctx, logger, addr, handler)
		if err != nil || shutdown == nil {
			return nil, err
		}

		return metricsShutdownFunc(shutdown), nil
	}
}

// WithControllerFactory overrides controller construction.
func (r *IntegrationRunner) WithControllerFactory(
	factory func(
		context.Context,
		string,
		runtimeconfig.Config,
		imds.Client,
		adapt.MetricsRecorder,
	) (adapt.Controller, PoolStarter, error),
) {
	r.deps.newController = factory
}

// WithDetectCgroup sets the cgroup detection seam.
func (r *IntegrationRunner) WithDetectCgroup(detector func() (*cgroup.CPU, error)) {
	r.deps.detectCgroup = detector
}

// Run executes the application with the configured overrides.
func (r *IntegrationRunner) Run(ctx context.Context, args []string, stderr io.Writer) int {
	return run(ctx, args, r.deps, stderr)
}
