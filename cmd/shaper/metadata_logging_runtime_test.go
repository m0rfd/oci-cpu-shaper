package main

import (
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

const runtimeMetadataIncompleteMsg = "runtime metadata incomplete"

type metadataResolutionExpectation struct {
	name           string
	mode           string
	metadata       ociMetadata
	offline        bool
	expectedMsg    string
	expectedLevel  zapcore.Level
	expectedFields map[string]string
	expectOffline  *bool
	missingFields  []string
}

func boolPtr(value bool) *bool {
	return &value
}

var metadataResolutionCases = []metadataResolutionExpectation{ //nolint:gochecknoglobals
	{
		name:           "noop mode skips resolution",
		mode:           modeNoop,
		metadata:       ociMetadata{CompartmentID: "", Region: ""},
		offline:        false,
		expectedMsg:    "metadata resolution skipped",
		expectedLevel:  zap.DebugLevel,
		expectedFields: map[string]string{"mode": modeNoop},
		expectOffline:  nil,
		missingFields:  []string{"offline", "compartmentID", "region"},
	},
	{
		name:           "offline metadata logs info",
		mode:           modeEnforce,
		metadata:       ociMetadata{CompartmentID: "  " + stubCompartmentID + "  ", Region: ""},
		offline:        true,
		expectedMsg:    "using offline metadata configuration",
		expectedLevel:  zap.InfoLevel,
		expectedFields: map[string]string{"compartmentID": stubCompartmentID},
		expectOffline:  boolPtr(true),
		missingFields:  []string{"region"},
	},
	{
		name:           "warns when compartment missing",
		mode:           modeEnforce,
		metadata:       ociMetadata{CompartmentID: "", Region: "  " + stubRegion + "  "},
		offline:        false,
		expectedMsg:    runtimeMetadataIncompleteMsg,
		expectedLevel:  zap.WarnLevel,
		expectedFields: map[string]string{"region": stubRegion},
		expectOffline:  boolPtr(false),
		missingFields:  []string{"compartmentID"},
	},
	{
		name:           "warns when region missing",
		mode:           modeEnforce,
		metadata:       ociMetadata{CompartmentID: "  " + stubCompartmentID + "  ", Region: ""},
		offline:        false,
		expectedMsg:    runtimeMetadataIncompleteMsg,
		expectedLevel:  zap.WarnLevel,
		expectedFields: map[string]string{"compartmentID": stubCompartmentID},
		expectOffline:  boolPtr(false),
		missingFields:  []string{"region"},
	},
	{
		name: "logs info when metadata fully resolved",
		mode: modeDryRun,
		metadata: ociMetadata{
			CompartmentID: "\n" + stubCompartmentID,
			Region:        "\t" + stubRegion,
		},
		offline:        false,
		expectedMsg:    "resolved runtime metadata",
		expectedLevel:  zap.InfoLevel,
		expectedFields: map[string]string{"compartmentID": stubCompartmentID, "region": stubRegion},
		expectOffline:  boolPtr(false),
		missingFields:  nil,
	},
}

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

func TestLogMetadataResolutionLogsWithObserver(t *testing.T) {
	t.Parallel()

	for _, testCase := range metadataResolutionCases {
		tc := testCase

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assertMetadataResolutionLog(t, tc)
		})
	}
}

func assertMetadataResolutionLog(t *testing.T, test metadataResolutionExpectation) {
	t.Helper()

	logger, observed := newObservedLogger(zap.DebugLevel)

	logMetadataResolution(logger, test.mode, test.metadata, test.offline)

	entry := singleObservedEntry(t, observed)
	assertLogLevelAndMessage(t, entry, test.expectedLevel, test.expectedMsg)
	assertOfflineField(t, entry, test.expectOffline)
	assertExpectedFields(t, entry, test.expectedFields)
	assertMissingFields(t, entry, test.missingFields)
}

func singleObservedEntry(t *testing.T, observed *observer.ObservedLogs) observer.LoggedEntry {
	t.Helper()

	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("expected single log entry, got %+v", entries)
	}

	return entries[0]
}

func assertLogLevelAndMessage(
	t *testing.T,
	entry observer.LoggedEntry,
	expectedLevel zapcore.Level,
	expectedMsg string,
) {
	t.Helper()

	if entry.Level != expectedLevel {
		t.Fatalf("expected level %s, got %s", expectedLevel, entry.Level)
	}

	if entry.Message != expectedMsg {
		t.Fatalf("expected log message %q, got %q", expectedMsg, entry.Message)
	}
}

func assertOfflineField(t *testing.T, entry observer.LoggedEntry, expected *bool) {
	t.Helper()

	if expected == nil {
		if hasField(entry.Context, "offline") {
			t.Fatalf("did not expect offline field, got %+v", entry.Context)
		}

		return
	}

	offlineValue, ok := fieldBool(entry.Context, "offline")
	if !ok || offlineValue != *expected {
		t.Fatalf("expected offline=%v (ok=%v), got %v", *expected, ok, offlineValue)
	}
}

func assertExpectedFields(
	t *testing.T,
	entry observer.LoggedEntry,
	expectedFields map[string]string,
) {
	t.Helper()

	for key, value := range expectedFields {
		requireLogFieldString(t, entry, key, value)
	}
}

func assertMissingFields(t *testing.T, entry observer.LoggedEntry, missingFields []string) {
	t.Helper()

	for _, key := range missingFields {
		if hasField(entry.Context, key) {
			t.Fatalf("expected field %q to be absent, got %+v", key, entry.Context)
		}
	}
}
