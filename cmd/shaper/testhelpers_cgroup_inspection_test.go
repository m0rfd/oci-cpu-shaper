package main

import (
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
)

func newObservedLogger(level zapcore.LevelEnabler) (*zap.Logger, *observer.ObservedLogs) {
	core, observed := observer.New(level)

	return zap.New(core), observed
}

func renderExporter(t *testing.T, exporter *metricshttp.Exporter) string {
	t.Helper()

	data, err := exporter.Render()
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	return string(data)
}

func assertMetricContains(t *testing.T, metrics, want string) {
	t.Helper()

	if !strings.Contains(metrics, want) {
		t.Fatalf("expected metrics to contain %q, got %q", want, metrics)
	}
}
