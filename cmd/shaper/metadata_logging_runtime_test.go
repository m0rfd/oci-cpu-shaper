package main

import (
	"testing"
	"time"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/adapt"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

func TestLogRuntimeConfig(t *testing.T) {
	t.Parallel()

	logger, observed := newObservedLogger(zap.InfoLevel)

	cfg := runtimeconfig.Config{
		Controller: runtimeconfig.ControllerConfig{ //nolint:exhaustruct
			TargetMin:                 0.21,
			TargetMax:                 0.39,
			GoalLow:                   0.23,
			GoalHigh:                  0.30,
			Interval:                  time.Minute,
			RelaxedInterval:           6 * time.Hour,
			SuppressThreshold:         0.85,
			SuppressResume:            0.70,
			SuppressRunnableThreshold: 1.25,
			SuppressRunnableResume:    1.05,
		},
		Estimator: runtimeconfig.EstimatorConfig{Interval: 2 * time.Second},
		Pool: runtimeconfig.PoolConfig{
			Workers:           4,
			AutoSizeFromShape: false,
			Quantum:           50 * time.Millisecond,
			PauseThreshold:    0.85,
			ResumeThreshold:   0.70,
			RunnableGuard:     1.3,
		},
		HTTP: runtimeconfig.HTTPConfig{Bind: "127.0.0.1:9000"},
		OCI:  runtimeconfig.OCIConfig{Offline: true}, //nolint:exhaustruct
	}

	logRuntimeConfig(logger, cfg)

	entry := requireSingleEntry(t, observed, zap.InfoLevel)
	if entry.Message != "loaded runtime configuration" {
		t.Fatalf("unexpected log message: %q", entry.Message)
	}

	if workers, ok := fieldInt(entry.Context, "workerCount"); !ok || workers != 4 {
		t.Fatalf("expected workerCount 4, got %d (present=%v)", workers, ok)
	}

	if duration, ok := fieldDuration(entry.Context, "workerQuantum"); !ok ||
		duration != 50*time.Millisecond {
		t.Fatalf("expected worker quantum 50ms, got %v (present=%v)", duration, ok)
	}

	if offline, ok := fieldBool(entry.Context, "offline"); !ok || !offline {
		t.Fatalf("expected offline flag true, got %v (present=%v)", offline, ok)
	}

	requireLogFieldFloat(t, entry, "controllerTargetMin", 0.21)
	requireLogFieldFloat(t, entry, "controllerTargetMax", 0.39)
	requireLogFieldFloat(t, entry, "controllerGoalLow", 0.23)
	requireLogFieldFloat(t, entry, "controllerGoalHigh", 0.30)
	requireLogFieldFloat(t, entry, "suppressThreshold", 0.85)
	requireLogFieldFloat(t, entry, "suppressResume", 0.70)
	requireLogFieldFloat(t, entry, "suppressRunnableThreshold", 1.25)
	requireLogFieldFloat(t, entry, "suppressRunnableResume", 1.05)
	requireLogFieldFloat(t, entry, "poolRunnableGuard", 1.3)
	requireLogFieldString(t, entry, "httpBind", "127.0.0.1:9000")
}

func TestLogRuntimeConfigNoopWithoutLogger(t *testing.T) {
	t.Parallel()

	_, observed := newObservedLogger(zap.InfoLevel)

	cfg := runtimeconfig.Default()

	logRuntimeConfig(nil, cfg)

	if entries := observed.All(); len(entries) != 0 {
		t.Fatalf("expected no log entries when logger is nil, got %+v", entries)
	}
}

func TestLogRuntimeConfigMarksHTTPOff(t *testing.T) {
	t.Parallel()

	logger, observed := newObservedLogger(zap.InfoLevel)

	cfg := runtimeconfig.Default()
	cfg.HTTP.Bind = " \t"

	logRuntimeConfig(logger, cfg)

	entry := requireSingleEntry(t, observed, zap.InfoLevel)
	if httpEnabled, ok := fieldBool(entry.Context, "httpEnabled"); !ok || httpEnabled {
		t.Fatalf("expected httpEnabled false, got %v (present=%v)", httpEnabled, ok)
	}

	if hasField(entry.Context, "httpBind") {
		t.Fatalf("expected httpBind to be omitted when bind is empty, got %+v", entry.Context)
	}
}

func TestLogRuntimeConfigMarksHTTPOn(t *testing.T) {
	t.Parallel()

	logger, observed := newObservedLogger(zap.InfoLevel)

	cfg := runtimeconfig.Default()
	cfg.HTTP.Bind = " 0.0.0.0:8080 "

	logRuntimeConfig(logger, cfg)

	entry := requireSingleEntry(t, observed, zap.InfoLevel)
	if httpEnabled, ok := fieldBool(entry.Context, "httpEnabled"); !ok || !httpEnabled {
		t.Fatalf("expected httpEnabled true, got %v (present=%v)", httpEnabled, ok)
	}

	requireLogFieldString(t, entry, "httpBind", "0.0.0.0:8080")
}

func TestLogMetadataResolutionOnline(t *testing.T) {
	t.Parallel()

	logger, observed := newObservedLogger(zap.DebugLevel)

	logMetadataResolution(
		logger,
		modeDryRun,
		ociMetadata{CompartmentID: stubCompartmentID, Region: stubRegion},
		false,
	)

	entry := requireSingleEntry(t, observed, zap.InfoLevel)
	if entry.Message != "resolved runtime metadata" {
		t.Fatalf("unexpected log message: %q", entry.Message)
	}

	requireLogFieldString(t, entry, "compartmentID", stubCompartmentID)
	requireLogFieldString(t, entry, "region", stubRegion)

	if offline, ok := fieldBool(entry.Context, "offline"); !ok || offline {
		t.Fatalf("expected offline flag false, got %v (present=%v)", offline, ok)
	}
}

func TestLogMetadataResolutionOffline(t *testing.T) {
	t.Parallel()

	logger, observed := newObservedLogger(zap.DebugLevel)

	logMetadataResolution(
		logger,
		modeEnforce,
		ociMetadata{CompartmentID: stubCompartmentID}, //nolint:exhaustruct
		true,
	)

	entry := requireSingleEntry(t, observed, zap.InfoLevel)
	if entry.Message != "using offline metadata configuration" {
		t.Fatalf("unexpected log message: %q", entry.Message)
	}

	requireLogFieldString(t, entry, "compartmentID", stubCompartmentID)

	if offline, ok := fieldBool(entry.Context, "offline"); !ok || !offline {
		t.Fatalf("expected offline flag true, got %v (present=%v)", offline, ok)
	}
}

func TestLogMetadataResolutionWarnsWhenIncomplete(t *testing.T) {
	t.Parallel()

	logger, observed := newObservedLogger(zap.DebugLevel)

	var emptyMetadata ociMetadata
	logMetadataResolution(logger, modeEnforce, emptyMetadata, false)

	entry := requireSingleEntry(t, observed, zap.WarnLevel)
	if entry.Message != "runtime metadata incomplete" {
		t.Fatalf("unexpected log message: %q", entry.Message)
	}

	if offline, ok := fieldBool(entry.Context, "offline"); !ok || offline {
		t.Fatalf("expected offline flag false, got %v (present=%v)", offline, ok)
	}
}

func TestLogMetadataResolutionSkipsNoopMode(t *testing.T) {
	t.Parallel()

	logger, observed := newObservedLogger(zap.DebugLevel)

	var noopMetadata ociMetadata
	logMetadataResolution(logger, modeNoop, noopMetadata, false)

	entry := requireSingleEntry(t, observed, zap.DebugLevel)
	if entry.Message != "metadata resolution skipped" {
		t.Fatalf("unexpected log message: %q", entry.Message)
	}

	requireLogFieldString(t, entry, "mode", modeNoop)
}

func TestLogControllerInitialization(t *testing.T) {
	t.Parallel()

	logger, observed := newObservedLogger(zap.InfoLevel)

	cfg := runtimeconfig.Config{ //nolint:exhaustruct
		Pool: runtimeconfig.PoolConfig{
			Workers:           2,
			AutoSizeFromShape: false,
			Quantum:           25 * time.Millisecond,
			PauseThreshold:    0.85,
			ResumeThreshold:   0.70,
			RunnableGuard:     1.1,
		},
		Estimator: runtimeconfig.EstimatorConfig{
			Interval: 750 * time.Millisecond,
		},
		OCI: runtimeconfig.OCIConfig{ //nolint:exhaustruct
			CompartmentID: stubCompartmentID,
			Region:        stubRegion,
		},
	}

	ctrl := &stubController{mode: modeDryRun, state: adapt.StateFallback} //nolint:exhaustruct
	exporter := metricshttp.NewExporter()

	logControllerInitialization(logger, cfg, ctrl, exporter)

	entry := requireSingleEntry(t, observed, zap.InfoLevel)
	if entry.Message != "controller initialized" {
		t.Fatalf("unexpected log message: %q", entry.Message)
	}

	requireLogFieldString(t, entry, "mode", modeDryRun)
	requireLogFieldString(t, entry, "controllerState", adapt.StateFallback.String())
	requireLogFieldString(t, entry, "compartmentID", stubCompartmentID)
	requireLogFieldString(t, entry, "region", stubRegion)

	if enforcing, ok := fieldBool(entry.Context, "enforcingTargets"); !ok || enforcing {
		t.Fatalf("expected enforcingTargets false for dry-run, got %v (present=%v)", enforcing, ok)
	}

	if workers, ok := fieldInt(entry.Context, "workerCount"); !ok || workers != 2 {
		t.Fatalf("expected worker count 2, got %d (present=%v)", workers, ok)
	}

	if metricsEnabled, ok := fieldBool(entry.Context, "metricsEnabled"); !ok || !metricsEnabled {
		t.Fatalf("expected metricsEnabled true, got %v (present=%v)", metricsEnabled, ok)
	}
}
