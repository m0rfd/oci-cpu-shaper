package main

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"oci-cpu-shaper/pkg/adapt"
)

func TestLogIMDSMetadataEmitsDetails(t *testing.T) {
	t.Parallel()

	logger, observed := newObservedLogger(zap.DebugLevel)

	client := newLoggingStubIMDS(
		stubRegion,
		nil,
		stubRegion,
		nil,
		"ocid1.instance.oc1..exampleuniqueID",
		nil,
		stubCompartmentID,
		nil,
		stubShapeConfig(4, 64),
		nil,
	)

	ctrl := &stubController{mode: modeDryRun, state: adapt.StateSuppressed} //nolint:exhaustruct

	logIMDSMetadata(context.Background(), logger, client, ctrl, "", "", "", false)

	entry := requireSingleEntry(t, observed, zapcore.InfoLevel)
	requireLogFieldString(t, entry, "controllerMode", modeDryRun)
	requireLogFieldString(t, entry, "controllerState", adapt.StateSuppressed.String())
	requireLogFieldString(t, entry, "region", stubRegion)
	requireLogFieldString(t, entry, "canonicalRegion", stubRegion)
	requireLogFieldString(t, entry, "instanceID", "ocid1.instance.oc1..exampleuniqueID")
	requireLogFieldString(t, entry, "compartmentID", stubCompartmentID)
	requireLogFieldFloat(t, entry, "shapeOCPUs", 4)
	requireLogFieldFloat(t, entry, "shapeMemoryGB", 64)
}

func TestLogIMDSMetadataWarnsOnFailures(t *testing.T) {
	t.Parallel()

	logger, observed := newObservedLogger(zap.DebugLevel)

	client := newLoggingStubIMDS(
		"",
		errRegionDown,
		"",
		errRegionDown,
		"",
		errInstanceDown,
		"",
		errInstanceDown,
		stubShapeConfig(0, 0),
		errShapeDown,
	)

	ctrl := &stubController{mode: modeNoop, state: adapt.StateFallback} //nolint:exhaustruct

	logIMDSMetadata(context.Background(), logger, client, ctrl, "", "", "", false)

	warns := observed.FilterLevelExact(zapcore.WarnLevel).All()
	if len(warns) != 5 {
		t.Fatalf("expected five warnings, got %d", len(warns))
	}
}

func TestLogIMDSMetadataUsesOverrideInstanceID(t *testing.T) {
	t.Parallel()

	logger, observed := newObservedLogger(zap.DebugLevel)

	client := newLoggingStubIMDS(
		overrideRegion,
		nil,
		stubCanonicalRegion,
		nil,
		"",
		nil,
		stubCompartmentID,
		nil,
		stubShapeConfig(2, 32),
		nil,
	)

	ctrl := &stubController{mode: modeDryRun, state: adapt.StateNormal} //nolint:exhaustruct

	logIMDSMetadata(
		context.Background(),
		logger,
		client,
		ctrl,
		"  ocid1.instance.oc1..override  ",
		stubCompartmentID,
		overrideRegion,
		false,
	)

	requireOverrideIMDSLookups(t, client)

	entry := requireSingleEntry(t, observed, zapcore.InfoLevel)
	requireLogFieldString(t, entry, "controllerState", adapt.StateNormal.String())
	requireLogFieldString(t, entry, "instanceID", "ocid1.instance.oc1..override")
	requireLogFieldString(t, entry, "canonicalRegion", stubCanonicalRegion)
	requireLogFieldString(t, entry, "compartmentID", stubCompartmentID)

	warns := observed.FilterLevelExact(zapcore.WarnLevel).All()
	if len(warns) != 0 {
		t.Fatalf("expected no warnings, got %d", len(warns))
	}
}

func TestLogIMDSMetadataFallsBackToOverrideCanonicalRegion(t *testing.T) {
	t.Parallel()

	logger, observed := newObservedLogger(zap.DebugLevel)

	client := newLoggingStubIMDS(
		overrideRegion,
		nil,
		"",
		errRegionDown,
		"ocid1.instance.oc1..override",
		nil,
		stubCompartmentID,
		nil,
		stubShapeConfig(2, 16),
		nil,
	)

	ctrl := &stubController{mode: modeDryRun, state: adapt.StateNormal} //nolint:exhaustruct

	logIMDSMetadata(
		context.Background(),
		logger,
		client,
		ctrl,
		"  ocid1.instance.oc1..override  ",
		stubCompartmentID,
		overrideRegion,
		false,
	)

	requireOverrideIMDSLookups(t, client)

	entry := requireSingleEntry(t, observed, zapcore.InfoLevel)
	requireLogFieldString(t, entry, "controllerState", adapt.StateNormal.String())
	requireLogFieldString(t, entry, "region", overrideRegion)
	requireLogFieldString(t, entry, "canonicalRegion", overrideRegion)
	requireLogFieldString(t, entry, "instanceID", "ocid1.instance.oc1..override")
	requireLogFieldString(t, entry, "compartmentID", stubCompartmentID)

	warns := observed.FilterLevelExact(zapcore.WarnLevel).All()
	if len(warns) != 1 {
		t.Fatalf("expected single warning, got %d", len(warns))
	}
}

func TestLogIMDSMetadataOfflineSkipsIMDS(t *testing.T) {
	t.Parallel()

	logger, observed := newObservedLogger(zap.DebugLevel)

	client := newOfflineStubIMDS()
	ctrl := &stubController{
		mode:        modeEnforce,
		runErr:      nil,
		runCalled:   false,
		deadline:    time.Time{},
		deadlineSet: false,
		state:       adapt.StateNormal,
		lastErr:     nil,
		estErr:      nil,
	}

	logIMDSMetadata(
		context.Background(),
		logger,
		client,
		ctrl,
		"  ocid1.instance.oc1..offline  ",
		"",
		"",
		true,
	)

	assertNoIMDSCalls(t, client)
	assertOfflineLog(t, observed, "ocid1.instance.oc1..offline")
}
