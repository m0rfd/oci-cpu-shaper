package main

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/adapt"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
)

func TestConfigureMetricsSkipsExporterWhenMissing(t *testing.T) {
	t.Parallel()

	handler := configureMetrics(zap.NewNop(), nil, nil, nil, nil)
	if handler != nil {
		t.Fatal("expected handler to be nil when exporter is missing")
	}
}

func TestConfigureMetricsSetsWorkerMetrics(t *testing.T) {
	t.Parallel()

	exporter := metricshttp.NewExporter()
	pool := &stubPoolStarter{startCount: 0, workers: 3, quantum: 150 * time.Millisecond}

	_ = configureMetrics(zap.NewNop(), exporter, pool, nil, nil)

	snapshot, err := exporter.Render()
	if err != nil {
		t.Fatalf("render metrics: %v", err)
	}

	if !bytes.Contains(snapshot, []byte("worker_count 3")) {
		t.Fatalf("expected worker count metric, got %s", snapshot)
	}

	if !bytes.Contains(snapshot, []byte("duty_cycle_ms 150.000")) {
		t.Fatalf("expected duty cycle metric, got %s", snapshot)
	}
}

func TestConfigureMetricsRegistersHandlers(t *testing.T) {
	t.Parallel()

	exporter := metricshttp.NewExporter()
	pool := &stubPoolStarter{startCount: 0, workers: 5, quantum: 200 * time.Millisecond}
	controller := &stubController{
		mode:        modeDryRun,
		runErr:      nil,
		runCalled:   false,
		deadline:    time.Time{},
		deadlineSet: false,
		state:       adapt.StateFallback,
		lastErr:     errStubControllerRun,
		estErr:      errStubQueryFailure,
	}

	handler := configureMetrics(zap.NewNop(), exporter, pool, controller, nil)
	if handler == nil {
		t.Fatal("expected handler to be configured")
	}

	metricsRecorder := serveGETRequest(t, handler, "/metrics")
	if metricsRecorder.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected metrics response 200, got %d", metricsRecorder.Result().StatusCode)
	}

	metricsBody := metricsRecorder.Body.Bytes()
	if !bytes.Contains(metricsBody, []byte("worker_count 5")) {
		t.Fatalf("expected metrics to include worker count, got %s", metricsBody)
	}

	healthRecorder := serveGETRequest(t, handler, "/healthz")
	if healthRecorder.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected health status 200, got %d", healthRecorder.Result().StatusCode)
	}

	healthBody := healthRecorder.Body.Bytes()
	if !bytes.Contains(healthBody, []byte("\"mode\":\"dry-run\"")) {
		t.Fatalf("expected controller mode in health response, got %s", healthBody)
	}

	if !bytes.Contains(healthBody, []byte("\"state\":\"fallback\"")) {
		t.Fatalf("expected fallback state in health response, got %s", healthBody)
	}

	if !bytes.Contains(healthBody, []byte(errStubControllerRun.Error())) {
		t.Fatalf("expected controller error in health response, got %s", healthBody)
	}

	if !bytes.Contains(healthBody, []byte(errStubQueryFailure.Error())) {
		t.Fatalf("expected estimator error in health response, got %s", healthBody)
	}
}

func TestConfigureMetricsWithoutController(t *testing.T) {
	t.Parallel()

	exporter := metricshttp.NewExporter()

	handler := configureMetrics(zap.NewNop(), exporter, nil, nil, nil)
	if handler == nil {
		t.Fatal("expected handler to be configured")
	}

	recorder := serveGETRequest(t, handler, "/healthz")

	if recorder.Result().StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing health handler, got %d", recorder.Result().StatusCode)
	}
}

func TestConfigureMetricsServesPrometheusText(t *testing.T) {
	t.Parallel()

	exporter := metricshttp.NewExporter()
	exporter.SetMode("enforce")
	exporter.SetState("normal")
	exporter.SetTarget(0.42)
	exporter.ObserveOCIP95(0.31, time.Unix(1_700_000_333, 0))
	exporter.ObserveHostCPU(0.55)

	pool := &stubPoolStarter{startCount: 0, workers: 3, quantum: 2 * time.Millisecond}
	controller := &stubController{
		mode:        "enforce",
		runErr:      nil,
		runCalled:   false,
		deadline:    time.Time{},
		deadlineSet: false,
		state:       adapt.StateNormal,
		lastErr:     nil,
		estErr:      nil,
	}

	handler := configureMetrics(zap.NewNop(), exporter, pool, controller, nil)
	if handler == nil {
		t.Fatal("expected handler to be configured")
	}

	recorder := serveGETRequest(t, handler, "/metrics")

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", recorder.Code)
	}

	const promContentType = "application/openmetrics-text; version=1.0.0; charset=utf-8"
	if got := recorder.Header().Get("Content-Type"); got != promContentType {
		t.Fatalf("expected Prometheus content type, got %q", got)
	}

	body := recorder.Body.String()
	for _, snippet := range []string{
		"# HELP shaper_target_ratio",
		`shaper_mode{mode="enforce"} 1`,
		"shaper_enforcing 1",
		"worker_count 3",
		"duty_cycle_ms 2.000",
		"oci_last_success_epoch 1700000333",
	} {
		if !strings.Contains(body, snippet) {
			t.Fatalf("expected metrics output to contain %q, got:\n%s", snippet, body)
		}
	}
}
