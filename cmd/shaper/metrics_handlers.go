package main

import (
	"net/http"
	"time"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/cgroup"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
	statushttp "oci-cpu-shaper/pkg/http/status"
)

type metricsExporter interface {
	http.Handler

	SetWorkerCount(count int)
	SetDutyCycle(duration time.Duration)
}

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
	exporter metricsExporter,
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
