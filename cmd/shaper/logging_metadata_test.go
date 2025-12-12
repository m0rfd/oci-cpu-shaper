package main

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
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
