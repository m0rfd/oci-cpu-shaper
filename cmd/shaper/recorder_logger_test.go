package main

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
)

var (
	errMonitoringUnavailable = errors.New("monitoring unavailable")
	errEstimatorFailure      = errors.New("estimator error")
)

func TestNewRecorderLoggerHandlesNilInputs(t *testing.T) {
	t.Parallel()

	exporter := metricshttp.NewExporter()

	if recorder := newRecorderLogger(nil, exporter); recorder != exporter {
		t.Fatal("expected delegate to be returned when logger is nil")
	}

	logger := zap.NewNop()
	if recorder := newRecorderLogger(logger, nil); recorder != nil {
		t.Fatal("expected nil recorder when delegate is missing")
	}

	recorder := newRecorderLogger(logger, exporter)
	if recorder == exporter {
		t.Fatal("expected logging recorder wrapper when logger and delegate are provided")
	}
}

func TestControllerRecorderLoggerSetModeLogsChanges(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	recorder := newRecorderLogger(logger, metricshttp.NewExporter())

	recorder.SetMode("dry-run")
	recorder.SetMode("dry-run")
	recorder.SetMode("enforce")

	entries := observed.Filter(func(entry observer.LoggedEntry) bool {
		return entry.Message == "controller mode configured"
	}).All()

	if len(entries) != 2 {
		t.Fatalf("expected two mode configuration logs, got %d", len(entries))
	}

	if got := fieldString(entries[0].Context, "mode"); got != "dry-run" {
		t.Fatalf("expected initial mode log to capture dry-run, got %q", got)
	}

	if got := fieldString(entries[1].Context, "mode"); got != "enforce" {
		t.Fatalf("expected second mode log to capture enforce, got %q", got)
	}
}

func TestControllerRecorderLoggerSetModeNormalizesEmptyValues(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	recorder := newRecorderLogger(logger, metricshttp.NewExporter())

	recorder.SetMode("   ")

	entries := observed.Filter(func(entry observer.LoggedEntry) bool {
		return entry.Message == "controller mode configured"
	}).All()

	if len(entries) != 1 {
		t.Fatalf("expected a single mode log, got %d", len(entries))
	}

	if got := fieldString(entries[0].Context, "mode"); got != controllerUnknownValue {
		t.Fatalf("expected empty mode to normalize to unknown, got %q", got)
	}
}

func TestControllerRecorderLoggerSetStateLogsTransitions(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	recorder := newRecorderLogger(logger, metricshttp.NewExporter())

	recorder.SetState("normal")
	recorder.SetState("fallback")
	recorder.SetState("fallback")

	entries := observed.FilterLevelExact(zap.InfoLevel).All()
	if len(entries) != 2 {
		t.Fatalf("expected two info logs for state changes, got %d", len(entries))
	}

	first := entries[0]
	if first.Message != "controller state transition" {
		t.Fatalf("unexpected first message %q", first.Message)
	}

	if got := fieldString(first.Context, "from"); got != "" {
		t.Fatalf("expected first transition to originate from empty state, got %q", got)
	}

	if got := fieldString(first.Context, "to"); got != "normal" {
		t.Fatalf("expected transition to normal, got %q", got)
	}

	second := entries[1]
	if got := fieldString(second.Context, "from"); got != "normal" {
		t.Fatalf("expected fallback transition to originate from normal, got %q", got)
	}

	if got := fieldString(second.Context, "to"); got != "fallback" {
		t.Fatalf("expected fallback transition, got %q", got)
	}
}

func TestControllerRecorderLoggerSetStateNormalizesEmptyValues(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	recorder := newRecorderLogger(logger, metricshttp.NewExporter())

	recorder.SetState("   ")

	entries := observed.FilterLevelExact(zap.InfoLevel).All()

	if len(entries) != 1 {
		t.Fatalf("expected single state log for empty input, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Message != "controller state transition" {
		t.Fatalf("unexpected log message %q", entry.Message)
	}

	if got := fieldString(entry.Context, "to"); got != controllerUnknownValue {
		t.Fatalf("expected empty state to normalize to unknown, got %q", got)
	}
}

func TestControllerRecorderLoggerSetTargetLogsWithThreshold(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	recorder := newRecorderLogger(logger, metricshttp.NewExporter())

	recorder.SetTarget(0.25)
	recorder.SetTarget(0.253)
	recorder.SetTarget(0.33)

	entries := observed.FilterLevelExact(zap.DebugLevel).All()
	if len(entries) != 2 {
		t.Fatalf("expected two debug logs for target updates, got %d", len(entries))
	}

	first := entries[0]
	if got := fieldFloat(first.Context, "target"); got != 0.25 {
		t.Fatalf("expected first target log to capture 0.25, got %f", got)
	}

	second := entries[1]
	if got := fieldFloat(second.Context, "target"); got != 0.33 {
		t.Fatalf("expected final target log to capture 0.33, got %f", got)
	}
}

func TestControllerRecorderLoggerSetIntervalLogsChanges(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	recorder := newRecorderLogger(logger, metricshttp.NewExporter())

	recorder.SetInterval(30 * time.Second)
	recorder.SetInterval(30 * time.Second)
	recorder.SetInterval(time.Minute)

	entries := observed.FilterMessage(controllerIntervalLogMessage).All()
	if len(entries) != 2 {
		t.Fatalf("expected two interval logs, got %d", len(entries))
	}

	if interval, ok := fieldDuration(entries[0].Context, "interval"); !ok ||
		interval != 30*time.Second {
		t.Fatalf("expected first interval log to capture 30s, got %v (ok=%v)", interval, ok)
	}

	if interval, ok := fieldDuration(entries[1].Context, "interval"); !ok ||
		interval != time.Minute {
		t.Fatalf("expected second interval log to capture 1m, got %v (ok=%v)", interval, ok)
	}
}

func TestControllerRecorderLoggerSetLastErrorLogsTransitions(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	recorder := newRecorderLogger(logger, metricshttp.NewExporter())

	recorder.SetLastError(errMonitoringUnavailable)
	recorder.SetLastError(errMonitoringUnavailable)
	recorder.SetLastError(errEstimatorFailure)
	recorder.SetLastError(nil)

	entries := observed.All()
	if len(entries) != 3 {
		t.Fatalf("expected three logs (two errors + clear), got %d", len(entries))
	}

	first := entries[0]
	if first.Message != controllerErrorObservedMessage {
		t.Fatalf("expected first log to report observed error, got %q", first.Message)
	}

	if got := fieldString(first.Context, "error"); got != "monitoring unavailable" {
		t.Fatalf("unexpected error label %q", got)
	}

	second := entries[1]
	if got := fieldString(second.Context, "error"); got != "estimator error" {
		t.Fatalf("expected second error log to capture estimator error, got %q", got)
	}

	third := entries[2]
	if third.Message != controllerErrorClearedMessage {
		t.Fatalf("expected final log to clear error, got %q", third.Message)
	}
}

func TestControllerRecorderLoggerObserveOCIP95UsesCooldown(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	recorder := newRecorderLogger(logger, metricshttp.NewExporter())

	impl, ok := recorder.(*controllerRecorderLogger)
	if !ok {
		t.Fatal("expected recorder to be controllerRecorderLogger")
	}

	base := time.Date(2024, time.July, 10, 12, 0, 0, 0, time.UTC)

	impl.now = func() time.Time { return base }

	recorder.ObserveOCIP95(0.2, base)

	impl.now = func() time.Time { return base.Add(10 * time.Second) }

	recorder.ObserveOCIP95(0.205, base.Add(10*time.Second))

	impl.now = func() time.Time { return base.Add(controllerObservationCooldown + time.Second) }

	recorder.ObserveOCIP95(0.205, base.Add(controllerObservationCooldown+time.Second))

	entries := observed.Filter(func(entry observer.LoggedEntry) bool {
		return entry.Message == "oci metrics observation"
	}).All()

	if len(entries) != 2 {
		t.Fatalf("expected two OCI observation logs, got %d", len(entries))
	}

	if got := fieldFloat(entries[0].Context, "p95"); got != 0.2 {
		t.Fatalf("expected initial observation to log 0.2, got %f", got)
	}

	if got := fieldFloat(entries[1].Context, "p95"); got != 0.205 {
		t.Fatalf("expected cooldown-driven observation to log 0.205, got %f", got)
	}
}

func TestControllerRecorderLoggerObserveOCIP95LogsWhenTimestampMissing(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	recorder := newRecorderLogger(logger, metricshttp.NewExporter())

	impl, ok := recorder.(*controllerRecorderLogger)
	if !ok {
		t.Fatal("expected recorder to be controllerRecorderLogger")
	}

	impl.ociLogged = true
	impl.lastOCI = 0.2
	impl.lastOCILog = time.Time{}
	impl.now = func() time.Time { return time.Date(2024, time.July, 10, 12, 0, 0, 0, time.UTC) }

	recorder.ObserveOCIP95(0.203, time.Time{})

	entries := observed.Filter(func(entry observer.LoggedEntry) bool {
		return entry.Message == "oci metrics observation"
	}).All()

	if len(entries) != 1 {
		t.Fatalf("expected single OCI observation log, got %d", len(entries))
	}

	if got := fieldFloat(entries[0].Context, "p95"); got != 0.203 {
		t.Fatalf("expected log to capture observation value, got %f", got)
	}
}

func TestControllerRecorderLoggerObserveHostCPULogsThresholds(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	recorder := newRecorderLogger(logger, metricshttp.NewExporter())

	impl, ok := recorder.(*controllerRecorderLogger)
	if !ok {
		t.Fatal("expected recorder to be controllerRecorderLogger")
	}

	base := time.Date(2024, time.July, 10, 12, 0, 0, 0, time.UTC)

	impl.now = func() time.Time { return base }

	recorder.ObserveHostCPU(0.10)

	impl.now = func() time.Time { return base.Add(5 * time.Second) }

	recorder.ObserveHostCPU(0.12)

	impl.now = func() time.Time { return base.Add(5*time.Second + controllerObservationCooldown) }

	recorder.ObserveHostCPU(0.12)

	entries := observed.Filter(func(entry observer.LoggedEntry) bool {
		return entry.Message == hostCPUObservationMessage
	}).All()

	if len(entries) != 2 {
		t.Fatalf("expected two host CPU logs, got %d", len(entries))
	}

	firstPercent := fieldFloat(entries[0].Context, "percent")
	if firstPercent != 10 {
		t.Fatalf("expected first host CPU log to capture 10%%, got %f", firstPercent)
	}

	secondPercent := fieldFloat(entries[1].Context, "percent")
	if secondPercent != 12 {
		t.Fatalf("expected host CPU cooldown log to capture 12%%, got %f", secondPercent)
	}
}

func TestControllerRecorderLoggerObserveHostCPULogsLargeDelta(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	recorder := newRecorderLogger(logger, metricshttp.NewExporter())

	recorder.ObserveHostCPU(0.10)
	recorder.ObserveHostCPU(0.22)

	entries := observed.Filter(func(entry observer.LoggedEntry) bool {
		return entry.Message == hostCPUObservationMessage
	}).All()

	if len(entries) != 2 {
		t.Fatalf(
			"expected large delta to trigger second log without cooldown, got %d entries",
			len(entries),
		)
	}

	if got := fieldFloat(entries[1].Context, "percent"); got != 22 {
		t.Fatalf("expected second log to capture 22%%, got %f", got)
	}
}

func TestControllerRecorderLoggerShouldLogObservationFallsBackToTimeNow(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	recorder := newRecorderLogger(logger, metricshttp.NewExporter())

	impl, ok := recorder.(*controllerRecorderLogger)
	if !ok {
		t.Fatal("expected recorder to be controllerRecorderLogger")
	}

	impl.now = nil
	past := time.Now().Add(-controllerObservationCooldown - time.Second)

	if !impl.shouldLogObservation(true, 0.2, 0.2, past, controllerHostObservationDelta) {
		t.Fatal("expected time.Now fallback to allow logging when cooldown elapsed")
	}
}
