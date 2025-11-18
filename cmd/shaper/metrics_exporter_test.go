package main

import (
	"testing"

	metricshttp "oci-cpu-shaper/pkg/http/metrics"
)

func TestBuildMetricsExporterUsesOverride(t *testing.T) {
	t.Parallel()

	expected := metricshttp.NewExporter()
	deps := defaultRunDeps()
	deps.newMetricsExporter = func() *metricshttp.Exporter {
		return expected
	}

	exporter := buildMetricsExporter(deps)
	if exporter != expected {
		t.Fatalf("expected override exporter, got %p", exporter)
	}
}

func TestBuildMetricsExporterFallsBackToDefault(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	deps.newMetricsExporter = func() *metricshttp.Exporter {
		return nil
	}

	exporter := buildMetricsExporter(deps)
	if exporter == nil {
		t.Fatal("expected fallback exporter, got nil")
	}
}

func TestBuildMetricsExporterDefaultWithoutOverride(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	deps.newMetricsExporter = nil

	exporter := buildMetricsExporter(deps)
	if exporter == nil {
		t.Fatal("expected default exporter when override is absent")
	}
}
