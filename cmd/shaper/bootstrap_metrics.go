package main

import (
	"context"
	"net/http"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/cgroup"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

type metricsBootstrap struct {
	exporter       *metricshttp.Exporter
	shutdown       metricsShutdownFunc
	cancel         context.CancelFunc
	cpuInfo        *cgroup.CPU
	metricsHandler http.Handler
}

func stageMetrics(
	ctx context.Context,
	deps runDeps,
	logger *zap.Logger,
	cfg runtimeconfig.Config,
	pool poolStarter,
	controller adapt.Controller,
	exporter *metricshttp.Exporter,
) (metricsBootstrap, error) {
	if exporter == nil {
		exporter = buildMetricsExporter(deps)
	}

	cpuInfo := detectAndReportCgroup(deps, logger, exporter)
	handler := configureMetrics(logger, exporter, pool, controller, cpuInfo)

	shutdown, cancel, metricsErr := startMetricsEndpoint(ctx, deps, logger, cfg.HTTP.Bind, handler)
	if metricsErr != nil {
		var empty metricsBootstrap

		return empty, metricsErr
	}

	return metricsBootstrap{
		exporter:       exporter,
		shutdown:       shutdown,
		cancel:         cancel,
		cpuInfo:        cpuInfo,
		metricsHandler: handler,
	}, nil
}
