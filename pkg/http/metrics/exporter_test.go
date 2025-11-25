package metrics_test

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	metrics "oci-cpu-shaper/pkg/http/metrics"
)

const openMetricsContentType = "application/openmetrics-text; version=1.0.0; charset=utf-8"

var errFailingWriter = errors.New("metrics: failing writer")

func TestExporterRenderProducesOpenMetrics(t *testing.T) {
	t.Parallel()

	exporter := metrics.NewExporter()
	exporter.SetMode(" dry-run ")
	exporter.SetState(" fallback ")
	exporter.SetTarget(0.275)
	exporter.ObserveOCIP95(0.33, time.Unix(1_700_001_234, 0))
	exporter.SetDutyCycle(1500 * time.Microsecond)

	body, err := exporter.Render()
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	got := string(body)
	expected := strings.Join([]string{
		"# HELP shaper_target_ratio Target duty cycle ratio assigned to worker pool.",
		"# TYPE shaper_target_ratio gauge",
		"shaper_target_ratio 0.275000",
		"# HELP shaper_mode Controller operating mode (value set to 1 for the active mode).",
		"# TYPE shaper_mode gauge",
		"shaper_mode{mode=\"dry-run\"} 1",
		"# HELP shaper_state Controller operating state (value set to 1 for the active state).",
		"# TYPE shaper_state gauge",
		"shaper_state{state=\"fallback\"} 1",
		"# HELP shaper_enforcing Controller enforcement status (1 when worker targets are applied).",
		"# TYPE shaper_enforcing gauge",
		"shaper_enforcing 0",
		"# HELP controller_interval_seconds Duration until the next controller step (seconds).",
		"# TYPE controller_interval_seconds gauge",
		"controller_interval_seconds 45.000000",
		"# HELP oci_api_p95_latency_ms P95 latency of OCI API calls observed by the controller.",
		"# TYPE oci_api_p95_latency_ms gauge",
		"oci_api_p95_latency_ms 0.330000",
		"# HELP oci_api_last_success_timestamp_seconds Timestamp of the last successful OCI API call.",
		"# TYPE oci_api_last_success_timestamp_seconds gauge",
		"oci_api_last_success_timestamp_seconds 1700001234.000000",
		"# HELP duty_cycle_ms Current duty cycle period in milliseconds.",
		"# TYPE duty_cycle_ms gauge",
		"duty_cycle_ms 1.500\n",
	}, "\n")

	if got != expected {
		t.Fatalf("unexpected metrics output:\nexpected:\n%s\n\nactual:\n%s", expected, got)
	}
}

func TestExporterRenderClampsInvalidMetrics(t *testing.T) {
	t.Parallel()

	exporter := metrics.NewExporter()

	samples := []struct {
		value     float64
		timestamp time.Time
	}{
		{value: math.NaN(), timestamp: time.Unix(1_700_000_100, 0)},
		{value: math.Inf(1), timestamp: time.Unix(1_700_000_200, 0)},
		{value: -0.5, timestamp: time.Unix(1_700_000_300, 0)},
	}

	for _, sample := range samples {
		exporter.ObserveOCIP95(sample.value, sample.timestamp)
	}

	body, err := exporter.Render()
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	output := string(body)

	expectations := map[string]string{
		"oci_api_p95_latency_ms":                 "oci_api_p95_latency_ms 0.000000",
		"oci_api_last_success_timestamp_seconds": "oci_api_last_success_timestamp_seconds 1700000300.000000",
	}

	for label, expected := range expectations {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %s to include %q, got %s", label, expected, output)
		}
	}
}

func TestExporterSetModeNoopDisablesEnforcement(t *testing.T) {
	t.Parallel()

	exporter := metrics.NewExporter()
	exporter.SetMode(" NoOp ")

	body, err := exporter.Render()
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	output := string(body)
	if !strings.Contains(output, "shaper_mode{mode=\"NoOp\"} 1") {
		t.Fatalf("expected noop mode metric in %s", output)
	}

	if !strings.Contains(output, "shaper_enforcing 0") {
		t.Fatalf("expected noop mode to report shaper_enforcing 0, got %s", output)
	}
}

func TestExporterObserveOCIP95TracksTimestamp(t *testing.T) {
	t.Parallel()

	exporter := metrics.NewExporter()
	timestamp := time.Unix(1_700_000_111, 0)
	exporter.ObserveOCIP95(0.45, timestamp)

	data, err := exporter.Render()
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	body := string(data)

	want := fmt.Sprintf("oci_api_last_success_timestamp_seconds %d.000000", timestamp.Unix())
	if !strings.Contains(body, want) {
		t.Fatalf("expected %q in metrics output, got %s", want, body)
	}
}

func TestExporterServeHTTPWritesContentType(t *testing.T) {
	t.Parallel()

	exporter := metrics.NewExporter()
	exporter.SetMode("noop")
	exporter.SetState("normal")

	recorder := httptest.NewRecorder()
	exporter.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if recorder.Code != 200 {
		t.Fatalf("unexpected status code: %d", recorder.Code)
	}

	if got := recorder.Header().Get("Content-Type"); got != openMetricsContentType {
		t.Fatalf("unexpected content type: %q", got)
	}
}

func TestExporterWriteToPropagatesWriterErrors(t *testing.T) {
	t.Parallel()

	exporter := metrics.NewExporter()
	exporter.SetMode("noop")

	_, err := exporter.WriteTo(failingWriter{})
	if err == nil {
		t.Fatal("expected error from WriteTo")
	}

	if !strings.Contains(err.Error(), "failing writer") {
		t.Fatalf("expected write error, got %v", err)
	}
}

func TestExporterWriteToRejectsNilWriter(t *testing.T) {
	t.Parallel()

	exporter := metrics.NewExporter()

	_, err := exporter.WriteTo(nil)
	if err == nil {
		t.Fatal("expected error when writer is nil")
	}

	if !strings.Contains(err.Error(), "writer is nil") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestExporterGuardsAgainstInvalidInputs(t *testing.T) {
	t.Parallel()

	exporter := metrics.NewExporter()
	exporter.SetMode("")
	exporter.SetState(" ")
	exporter.SetTarget(math.NaN())
	exporter.ObserveOCIP95(-10, time.Time{})
	exporter.SetDutyCycle(-time.Second)

	data, err := exporter.Render()
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	output := string(data)
	if !strings.Contains(output, "shaper_mode{mode=\"unknown\"} 1") {
		t.Fatalf("expected unknown enforcement mode, got %s", output)
	}

	if !strings.Contains(output, "shaper_state{state=\"unknown\"} 1") {
		t.Fatalf("expected unknown state, got %s", output)
	}

	if !strings.Contains(output, "shaper_enforcing 0") {
		t.Fatalf(
			"expected enforcing gauge to default to 0 (since unknown mode doesn't enforce), got %s",
			output,
		)
	}

	if !strings.Contains(output, "shaper_target_ratio 0.000000") {
		t.Fatalf("expected clamped target, got %s", output)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errFailingWriter
}
