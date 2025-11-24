package main

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/adapt"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

var errStageMetricsBoom = errors.New("boom")

func TestStageMetricsStartsServer(t *testing.T) {
	t.Parallel()

	exporter := metricshttp.NewExporter()
	started := false

	deps := defaultRunDeps()
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		started = true

		return func(context.Context) {}, nil
	}

	cfg := runtimeconfig.Default()
	cfg.HTTP.Bind = testMetricsBind
	cfg.Pool.Workers = 3
	cfg.Pool.Quantum = 100

	metricsStage, err := stageMetrics(
		context.Background(),
		deps,
		zap.NewNop(),
		cfg,
		&stubPoolStarter{startCount: 0, workers: 3, quantum: 100},
		adapt.NewNoopController(modeDryRun),
		exporter,
	)
	if err != nil {
		t.Fatalf("expected metrics stage to succeed, got %v", err)
	}

	if metricsStage.exporter != exporter {
		t.Fatalf("expected exporter to be reused, got %+v", metricsStage.exporter)
	}

	if metricsStage.shutdown == nil || metricsStage.cancel == nil {
		t.Fatalf(
			"expected metrics lifecycle hooks, got shutdown=%v cancel=%v",
			metricsStage.shutdown,
			metricsStage.cancel,
		)
	}

	if !started {
		t.Fatal("expected startMetricsServer to run")
	}
}

func TestStageMetricsPropagatesErrors(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		return nil, errStageMetricsBoom
	}

	cfg := runtimeconfig.Default()
	cfg.HTTP.Bind = testMetricsBind

	_, err := stageMetrics(
		context.Background(),
		deps,
		zap.NewNop(),
		cfg,
		nil,
		adapt.NewNoopController(modeDryRun),
		metricshttp.NewExporter(),
	)
	if err == nil {
		t.Fatal("expected metrics startup failure")
	}
}

func TestStageMetricsHandlesMissingExporter(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	deps.startMetricsServer = startMetricsServer

	cfg := runtimeconfig.Default()
	cfg.HTTP.Bind = testMetricsBind
	cfg.Pool.Workers = 2

	metricsStage, err := stageMetrics(
		context.Background(),
		deps,
		zap.NewNop(),
		cfg,
		nil,
		adapt.NewNoopController(modeDryRun),
		nil,
	)
	if err != nil {
		t.Fatalf("expected metrics stage to succeed, got %v", err)
	}

	if metricsStage.exporter == nil {
		t.Fatal("expected exporter to be created")
	}
}
