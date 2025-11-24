package metrics

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"oci-cpu-shaper/pkg/adapt"
)

const (
	contentType              = "application/openmetrics-text; version=1.0.0; charset=utf-8"
	millisecondsPerSecond    = 1000.0
	hundredPercent           = 100.0
	defaultErrorLabelNone    = "none"
	defaultLabelUnknown      = "unknown"
	defaultErrorLabelUnknown = defaultLabelUnknown
)

var (
	errNilWriter = errors.New("metrics: writer is nil")
	errNilBuffer = errors.New("metrics: buffer factory returned nil")
)

type byteBuffer interface {
	io.Writer
	Bytes() []byte
}

// Exporter tracks controller and estimator metrics and exposes them via HTTP.
type Exporter struct {
	mu sync.RWMutex

	shaperTarget    float64
	enforcementMode string
	shaperEnforcing float64
	controllerState string
	ociP95          float64
	ociLastSuccess  time.Time
	dutyCycleMillis float64
	workerCount     float64
	hostCPUPercent  float64
	intervalSeconds float64
	lastError       string
	cgroupWeight    float64
	cgroupMaxQuota  float64
	cgroupMaxPeriod float64
	cgroupMaxLimit  float64

	bufferFactory func() byteBuffer
}

// NewExporter constructs an Exporter with zeroed metrics.
func NewExporter() *Exporter {
	exporter := new(Exporter)
	exporter.bufferFactory = func() byteBuffer {
		return new(bytes.Buffer)
	}

	return exporter
}

// SetMode records the CLI enforcement mode label.
func (e *Exporter) SetMode(mode string) {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" {
		trimmed = defaultLabelUnknown
	}

	enforcement := 0.0
	if adapt.ModeEnforcesTargets(trimmed) {
		enforcement = 1
	}

	e.mu.Lock()
	e.enforcementMode = trimmed
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
	e.controllerState = trimmed
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

// SetInterval records the controller's next interval duration in seconds.
func (e *Exporter) SetInterval(duration time.Duration) {
	seconds := duration.Seconds()
	if seconds < 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		seconds = 0
	}

	e.mu.Lock()
	e.intervalSeconds = seconds
	e.mu.Unlock()
}

// SetLastError tracks the last controller error message.
func (e *Exporter) SetLastError(err error) {
	message := defaultErrorLabelNone
	if err != nil {
		message = strings.TrimSpace(err.Error())
		if message == "" {
			message = defaultErrorLabelUnknown
		}
	}

	e.mu.Lock()
	e.lastError = message
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

// SetWorkerCount records the number of active worker goroutines.
func (e *Exporter) SetWorkerCount(count int) {
	value := float64(count)
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		value = 0
	}

	e.mu.Lock()
	e.workerCount = value
	e.mu.Unlock()
}

// SetCgroupCPUWeight records the detected cgroup v2 cpu.weight value.
func (e *Exporter) SetCgroupCPUWeight(weight uint64) {
	value := float64(weight)
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		value = 0
	}

	e.mu.Lock()
	e.cgroupWeight = value
	e.mu.Unlock()
}

// SetCgroupCPUMax captures the configured cpu.max quota/period tuple.
// When unlimited is true, quota is ignored and a separate flag is toggled.
func (e *Exporter) SetCgroupCPUMax(quota uint64, period uint64, unlimited bool) {
	periodValue := float64(period)
	if periodValue < 0 || math.IsNaN(periodValue) || math.IsInf(periodValue, 0) {
		periodValue = 0
	}

	quotaValue := float64(quota)
	if quotaValue < 0 || math.IsNaN(quotaValue) || math.IsInf(quotaValue, 0) {
		quotaValue = 0
	}

	limit := 0.0
	if unlimited {
		limit = 1
		quotaValue = 0
	}

	e.mu.Lock()
	e.cgroupMaxQuota = quotaValue
	e.cgroupMaxPeriod = periodValue
	e.cgroupMaxLimit = limit
	e.mu.Unlock()
}

// ObserveHostCPU records the latest host CPU utilisation percentage.
func (e *Exporter) ObserveHostCPU(utilisation float64) {
	if math.IsNaN(utilisation) || math.IsInf(utilisation, 0) {
		utilisation = 0
	}

	if utilisation < 0 {
		utilisation = 0
	}

	percent := utilisation * hundredPercent
	if percent > hundredPercent {
		percent = hundredPercent
	}

	e.mu.Lock()
	e.hostCPUPercent = percent
	e.mu.Unlock()
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
	factory := e.bufferFactory
	if factory == nil {
		factory = func() byteBuffer { return new(bytes.Buffer) }
	}

	buffer := factory()
	if buffer == nil {
		return nil, errNilBuffer
	}

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

	lines := []string{
		"# HELP shaper_target_ratio Target duty cycle ratio assigned to worker pool.\n",
		"# TYPE shaper_target_ratio gauge\n",
		fmt.Sprintf("shaper_target_ratio %.6f\n", snapshot.shaperTarget),
		"# HELP shaper_mode Controller operating state (value set to 1 for the active state).\n",
		"# TYPE shaper_mode gauge\n",
		fmt.Sprintf("shaper_mode{state=\"%s\"} 1\n", snapshot.controllerState),
		"# HELP shaper_enforcement_mode CLI enforcement mode (value set to 1 for the active mode).\n",
		"# TYPE shaper_enforcement_mode gauge\n",
		fmt.Sprintf("shaper_enforcement_mode{mode=\"%s\"} 1\n", snapshot.enforcementMode),
		"# HELP shaper_enforcing Controller enforcement status (1 when worker targets are applied).\n",
		"# TYPE shaper_enforcing gauge\n",
		fmt.Sprintf("shaper_enforcing %.0f\n", snapshot.shaperEnforcing),
		"# HELP controller_interval_seconds Duration until the next controller step (seconds).\n",
		"# TYPE controller_interval_seconds gauge\n",
		fmt.Sprintf("controller_interval_seconds %.6f\n", snapshot.intervalSeconds),
		"# HELP controller_last_error_info Last controller error message (value set to 1 for the active error).\n",
		"# TYPE controller_last_error_info gauge\n",
		fmt.Sprintf("controller_last_error_info{error=%s} 1\n", strconv.Quote(snapshot.lastError)),
		"# HELP oci_p95 Last observed OCI CPU P95 ratio.\n",
		"# TYPE oci_p95 gauge\n",
		fmt.Sprintf("oci_p95 %.6f\n", snapshot.ociP95),
		"# HELP oci_last_success_epoch Unix epoch seconds of the last successful OCI metrics query.\n",
		"# TYPE oci_last_success_epoch counter\n",
		fmt.Sprintf("oci_last_success_epoch %.0f\n", snapshot.ociLastSuccessEpoch),
		"# HELP duty_cycle_ms Duty cycle quantum configured for workers (milliseconds).\n",
		"# TYPE duty_cycle_ms gauge\n",
		fmt.Sprintf("duty_cycle_ms %.3f\n", snapshot.dutyCycleMillis),
		"# HELP worker_count Number of worker goroutines consuming CPU.\n",
		"# TYPE worker_count gauge\n",
		fmt.Sprintf("worker_count %.0f\n", snapshot.workerCount),
		"# HELP host_cpu_percent Last recorded host CPU utilisation percentage.\n",
		"# TYPE host_cpu_percent gauge\n",
		fmt.Sprintf("host_cpu_percent %.2f\n", snapshot.hostCPUPercent),
		"# HELP cgroup_cpu_weight Detected cgroup v2 cpu.weight value for the process.\n",
		"# TYPE cgroup_cpu_weight gauge\n",
		fmt.Sprintf("cgroup_cpu_weight %.0f\n", snapshot.cgroupWeight),
		"# HELP cgroup_cpu_max_quota Detected cpu.max quota (microseconds). Zero when unlimited.\n",
		"# TYPE cgroup_cpu_max_quota gauge\n",
		fmt.Sprintf("cgroup_cpu_max_quota %.0f\n", snapshot.cgroupMaxQuota),
		"# HELP cgroup_cpu_max_period Detected cpu.max period (microseconds).\n",
		"# TYPE cgroup_cpu_max_period gauge\n",
		fmt.Sprintf("cgroup_cpu_max_period %.0f\n", snapshot.cgroupMaxPeriod),
		"# HELP cgroup_cpu_max_unlimited Flag set to 1 when cpu.max reports \"max\".\n",
		"# TYPE cgroup_cpu_max_unlimited gauge\n",
		fmt.Sprintf("cgroup_cpu_max_unlimited %.0f\n", snapshot.cgroupMaxLimit),
		"# EOF\n",
	}

	var total int64

	for _, line := range lines {
		n, err := io.WriteString(dst, line)

		total += int64(n)
		if err != nil {
			return total, fmt.Errorf("write metrics: %w", err)
		}
	}

	return total, nil
}

type exporterSnapshot struct {
	shaperTarget        float64
	enforcementMode     string
	shaperEnforcing     float64
	controllerState     string
	ociP95              float64
	ociLastSuccessEpoch float64
	dutyCycleMillis     float64
	workerCount         float64
	hostCPUPercent      float64
	intervalSeconds     float64
	lastError           string
	cgroupWeight        float64
	cgroupMaxQuota      float64
	cgroupMaxPeriod     float64
	cgroupMaxLimit      float64
}

func (e *Exporter) snapshot() exporterSnapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()

	epoch := 0.0
	if !e.ociLastSuccess.IsZero() {
		epoch = float64(e.ociLastSuccess.Unix())
	}

	errorLabel := e.lastError
	if strings.TrimSpace(errorLabel) == "" {
		errorLabel = defaultErrorLabelNone
	}

	controllerState := strings.TrimSpace(e.controllerState)
	if controllerState == "" {
		controllerState = defaultLabelUnknown
	}

	enforcementMode := strings.TrimSpace(e.enforcementMode)
	if enforcementMode == "" {
		enforcementMode = defaultLabelUnknown
	}

	return exporterSnapshot{
		shaperTarget:        e.shaperTarget,
		enforcementMode:     enforcementMode,
		shaperEnforcing:     e.shaperEnforcing,
		controllerState:     controllerState,
		ociP95:              e.ociP95,
		ociLastSuccessEpoch: epoch,
		dutyCycleMillis:     e.dutyCycleMillis,
		workerCount:         e.workerCount,
		hostCPUPercent:      e.hostCPUPercent,
		intervalSeconds:     e.intervalSeconds,
		lastError:           errorLabel,
		cgroupWeight:        e.cgroupWeight,
		cgroupMaxQuota:      e.cgroupMaxQuota,
		cgroupMaxPeriod:     e.cgroupMaxPeriod,
		cgroupMaxLimit:      e.cgroupMaxLimit,
	}
}
