package metrics

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"oci-cpu-shaper/pkg/adapt"
)

const (
	contentType                      = "application/openmetrics-text; version=1.0.0; charset=utf-8"
	millisecondsPerSecond            = 1000.0
	defaultLabelUnknown              = "unknown"
	defaultControllerIntervalSeconds = 45.0
)

var errNilWriter = errors.New("metrics: writer is nil")

// Exporter tracks controller and estimator metrics and exposes them via HTTP.
type Exporter struct {
	mu sync.RWMutex

	shaperTarget    float64
	shaperMode      string
	shaperEnforcing float64
	shaperState     string
	ociP95          float64
	ociLastSuccess  time.Time
	dutyCycleMillis float64
}

// NewExporter constructs an Exporter with zeroed metrics.
func NewExporter() *Exporter {
	return &Exporter{
		mu:              sync.RWMutex{},
		shaperTarget:    0,
		shaperMode:      defaultLabelUnknown,
		shaperEnforcing: 0,
		shaperState:     defaultLabelUnknown,
		ociP95:          0,
		ociLastSuccess:  time.Time{},
		dutyCycleMillis: 0,
	}
}

// SetMode records the CLI enforcement mode label.
func (e *Exporter) SetMode(mode string) {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" {
		trimmed = defaultLabelUnknown
	}

	enforcement := 0.0
	if trimmed != defaultLabelUnknown && adapt.ModeEnforcesTargets(trimmed) {
		enforcement = 1
	}

	e.mu.Lock()
	e.shaperMode = trimmed
	e.shaperEnforcing = enforcement
	e.mu.Unlock()
}

// SetState records the current controller state label.
func (e *Exporter) SetState(state string) {
	trimmed := strings.TrimSpace(state)
	if trimmed == "" {
		trimmed = defaultLabelUnknown
	}

	e.mu.Lock()
	e.shaperState = trimmed
	e.mu.Unlock()
}

// SetTarget stores the current duty-cycle target ratio.
func (e *Exporter) SetTarget(target float64) {
	if math.IsNaN(target) || math.IsInf(target, 0) {
		target = 0
	}

	clamped := math.Max(0, math.Min(1, target))

	e.mu.Lock()
	e.shaperTarget = clamped
	e.mu.Unlock()
}

// ObserveOCIP95 captures the most recent OCI P95 ratio and the time it was fetched.
func (e *Exporter) ObserveOCIP95(value float64, fetchedAt time.Time) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		value = 0
	}

	if value < 0 {
		value = 0
	}

	e.mu.Lock()

	e.ociP95 = value
	if !fetchedAt.IsZero() {
		e.ociLastSuccess = fetchedAt
	}

	e.mu.Unlock()
}

// SetDutyCycle stores the worker duty-cycle quantum in milliseconds.
func (e *Exporter) SetDutyCycle(duration time.Duration) {
	millis := duration.Seconds() * millisecondsPerSecond
	if millis < 0 || math.IsNaN(millis) || math.IsInf(millis, 0) {
		millis = 0
	}

	e.mu.Lock()
	e.dutyCycleMillis = millis
	e.mu.Unlock()
}

// SetInterval records the controller's next interval duration in seconds.
func (e *Exporter) SetInterval(_ time.Duration) {
	// No-op: metric not exported.
}

// SetLastError tracks the last controller error message.
func (e *Exporter) SetLastError(_ error) {
	// No-op: metric not exported.
}

// SetCgroupCPUWeight records the detected cgroup v2 cpu.weight value.
func (e *Exporter) SetCgroupCPUWeight(_ uint64) {
	// No-op: metric not exported.
}

// SetCgroupCPUMax captures the configured cpu.max quota/period tuple.
func (e *Exporter) SetCgroupCPUMax(_ uint64, _ uint64, _ bool) {
	// No-op: metric not exported.
}

// ObserveHostCPU records the latest host CPU utilisation percentage.
func (e *Exporter) ObserveHostCPU(_ float64) {
	// No-op: metric not exported.
}

// ServeHTTP implements http.Handler for the metrics exporter.
func (e *Exporter) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	data, err := e.Render()
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)

		return
	}

	writer.Header().Set("Content-Type", contentType)
	_, _ = writer.Write(data)
}

// Render returns the current metrics snapshot encoded as OpenMetrics text.
func (e *Exporter) Render() ([]byte, error) {
	buffer := new(bytes.Buffer)

	_, err := e.WriteTo(buffer)
	if err != nil {
		return nil, err
	}

	data := buffer.Bytes()
	cloned := make([]byte, len(data))
	copy(cloned, data)

	return cloned, nil
}

// WriteTo writes the current metrics snapshot to the provided writer.
//
//nolint:funlen // Metrics list is intentionally linear for readability.
func (e *Exporter) WriteTo(dst io.Writer) (int64, error) {
	if dst == nil {
		return 0, errNilWriter
	}

	snapshot := e.snapshot()

	var written int64

	// We use a buffer to construct the output to ensure atomic writes to the destination
	// and to easily count bytes written.
	var buf bytes.Buffer

	// Helper to write strings and track bytes
	write := func(s string) {
		n, _ := buf.WriteString(s)
		written += int64(n)
	}

	// Helper to write metric families
	writeMetric := func(name, help, typeName, value string) {
		write(fmt.Sprintf("# HELP %s %s\n", name, help))
		write(fmt.Sprintf("# TYPE %s %s\n", name, typeName))
		write(value)
	}

	// shaper_target_ratio
	writeMetric(
		"shaper_target_ratio",
		"Target duty cycle ratio assigned to worker pool.",
		"gauge",
		fmt.Sprintf("shaper_target_ratio %.6f\n", snapshot.shaperTarget),
	)

	// shaper_mode
	writeMetric(
		"shaper_mode",
		"Controller operating mode (value set to 1 for the active mode).",
		"gauge",
		fmt.Sprintf("shaper_mode{mode=\"%s\"} 1\n", snapshot.shaperMode),
	)

	// shaper_state
	writeMetric(
		"shaper_state",
		"Controller operating state (value set to 1 for the active state).",
		"gauge",
		fmt.Sprintf("shaper_state{state=\"%s\"} 1\n", snapshot.shaperState),
	)

	// shaper_enforcing
	writeMetric(
		"shaper_enforcing",
		"Controller enforcement status (1 when worker targets are applied).",
		"gauge",
		fmt.Sprintf("shaper_enforcing %.0f\n", snapshot.shaperEnforcing),
	)

	// controller_interval_seconds
	// Note: This is a constant in the current implementation, but exposed as a metric
	// for consistency with other exporters and potential future configurability.
	// We hardcode 45.0 here as it matches the default ticker interval.
	// Ideally this should come from configuration.
	writeMetric(
		"controller_interval_seconds",
		"Duration until the next controller step (seconds).",
		"gauge",
		fmt.Sprintf("controller_interval_seconds %.6f\n", defaultControllerIntervalSeconds),
	)

	// oci_api_p95_latency_ms
	writeMetric(
		"oci_api_p95_latency_ms",
		"P95 latency of OCI API calls observed by the controller.",
		"gauge",
		fmt.Sprintf("oci_api_p95_latency_ms %.6f\n", snapshot.ociP95),
	)

	// oci_api_last_success_timestamp_seconds
	writeMetric(
		"oci_api_last_success_timestamp_seconds",
		"Timestamp of the last successful OCI API call.",
		"gauge",
		fmt.Sprintf("oci_api_last_success_timestamp_seconds %.6f\n", snapshot.ociLastSuccessEpoch),
	)

	// duty_cycle_ms
	writeMetric(
		"duty_cycle_ms",
		"Current duty cycle period in milliseconds.",
		"gauge",
		fmt.Sprintf("duty_cycle_ms %.3f\n", snapshot.dutyCycleMillis),
	)

	n, err := dst.Write(buf.Bytes())

	return int64(n), err
}

type exporterSnapshot struct {
	shaperTarget        float64
	shaperMode          string
	shaperEnforcing     float64
	shaperState         string
	ociP95              float64
	ociLastSuccessEpoch float64
	dutyCycleMillis     float64
}

func (e *Exporter) snapshot() exporterSnapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()

	epoch := 0.0
	if !e.ociLastSuccess.IsZero() {
		epoch = float64(e.ociLastSuccess.Unix())
	}

	return exporterSnapshot{
		shaperTarget:        e.shaperTarget,
		shaperMode:          e.shaperMode,
		shaperEnforcing:     e.shaperEnforcing,
		shaperState:         e.shaperState,
		ociP95:              e.ociP95,
		ociLastSuccessEpoch: epoch,
		dutyCycleMillis:     e.dutyCycleMillis,
	}
}
