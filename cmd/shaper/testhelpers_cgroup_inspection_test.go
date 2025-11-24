package main

import (
	"strings"
	"testing"

	metricshttp "oci-cpu-shaper/pkg/http/metrics"
)

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
