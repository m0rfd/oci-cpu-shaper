package main

import (
	"context"
	"net/http"
	"strings"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/cgroup"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
	statushttp "oci-cpu-shaper/pkg/http/status"
)

type metricsShutdownFunc func(context.Context)

func buildMetricsExporter(deps runDeps) *metricshttp.Exporter {
	if deps.newMetricsExporter != nil {
		exporter := deps.newMetricsExporter()
		if exporter != nil {
			return exporter
		}
	}

	return metricshttp.NewExporter()
}

func configureMetrics(
	logger *zap.Logger,
	exporter *metricshttp.Exporter,
	pool poolStarter,
	controller adapt.Controller,
	cpuInfo *cgroup.CPU,
) http.Handler {
	if logger == nil {
		logger = zap.NewNop()
	}

	if exporter == nil {
		logger.Warn("metrics exporter disabled", zap.String("reason", "no exporter configured"))

		return nil
	}

	if pool != nil {
		workers := pool.Workers()
		exporter.SetWorkerCount(workers)

		quantum := pool.Quantum()
		exporter.SetDutyCycle(quantum)

		logger.Debug(
			"registered worker pool metrics",
			zap.Int("workerCount", workers),
			zap.Duration("dutyCycle", quantum),
		)
	} else {
		logger.Debug("worker pool metrics unavailable", zap.String("reason", "pool not configured"))
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", exporter)

	if controller != nil {
		mux.Handle("/healthz", statushttp.NewHandler(controller, cpuInfo))
	}

	return mux
}

func startMetricsEndpoint(
	ctx context.Context,
	deps runDeps,
	logger *zap.Logger,
	bindAddr string,
	handler http.Handler,
) (metricsShutdownFunc, context.CancelFunc, error) {
	if handler == nil {
		return nil, nil, nil
	}

	if deps.startMetricsServer == nil {
		logger.Warn("metrics server disabled", zap.String("reason", "start function missing"))

		return nil, nil, nil
	}

	trimmed := strings.TrimSpace(bindAddr)
	if trimmed == "" {
		logger.Info("metrics server disabled", zap.String("reason", "http bind address empty"))

		return nil, nil, nil
	}

	if ctx == nil {
		return nil, nil, errMetricsContextRequired
	}

	logger.Info("starting metrics server", zap.String("bind", trimmed))

	metricsCtx, cancel := context.WithCancel(ctx)

	shutdown, err := deps.startMetricsServer(metricsCtx, logger, trimmed, handler)
	if err != nil {
		cancel()

		return nil, nil, err
	}

	return shutdown, cancel, nil
}
