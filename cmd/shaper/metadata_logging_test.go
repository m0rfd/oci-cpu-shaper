package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"oci-cpu-shaper/pkg/adapt"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

func TestDefaultIMDSFactoryUsesEnvironmentEndpoint(t *testing.T) {
	responses := map[string]string{
		"/opc/v2/instance/region":      overrideRegion,
		"/opc/v2/instance/id":          "ocid1.instance.oc1..exampleuniqueID",
		"/opc/v2/instance/shapeConfig": `{"ocpus":2,"memoryInGBs":32}`,
	}

	server := newIPv4TestServer(
		t,
		http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
			body, ok := responses[req.URL.Path]
			if !ok {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}

			_, _ = writer.Write([]byte(body))
		}),
	)
	t.Cleanup(server.Close)

	t.Setenv(imdsEndpointEnv, " "+server.URL+"/opc/v2 ")

	client := defaultIMDSFactory()

	ctx := context.Background()

	region, err := client.Region(ctx)
	if err != nil {
		t.Fatalf("Region() returned error: %v", err)
	}

	if region != overrideRegion {
		t.Fatalf("unexpected region %q", region)
	}

	instanceID, err := client.InstanceID(ctx)
	if err != nil {
		t.Fatalf("InstanceID() returned error: %v", err)
	}

	if instanceID != "ocid1.instance.oc1..exampleuniqueID" {
		t.Fatalf("unexpected instance ID %q", instanceID)
	}

	shape, err := client.ShapeConfig(ctx)
	if err != nil {
		t.Fatalf("ShapeConfig() returned error: %v", err)
	}

	if shape.OCPUs != 2 || shape.MemoryInGBs != 32 {
		t.Fatalf("unexpected shape config: %+v", shape)
	}
}

func TestLogIMDSMetadataEmitsDetails(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)

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

	ctrl := new(stubController)
	ctrl.mode = modeDryRun
	ctrl.state = adapt.StateSuppressed

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

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)

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

	ctrl := new(stubController)
	ctrl.mode = modeNoop
	ctrl.state = adapt.StateFallback

	logIMDSMetadata(context.Background(), logger, client, ctrl, "", "", "", false)

	warns := observed.FilterLevelExact(zapcore.WarnLevel).All()
	if len(warns) != 5 {
		t.Fatalf("expected five warnings, got %d", len(warns))
	}
}

func TestLogIMDSMetadataUsesOverrideInstanceID(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)

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

	ctrl := new(stubController)
	ctrl.mode = modeDryRun
	ctrl.state = adapt.StateNormal

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

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)

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

	ctrl := new(stubController)
	ctrl.mode = modeDryRun
	ctrl.state = adapt.StateNormal

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

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)

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

func TestLogRuntimeConfig(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	cfg := runtimeconfig.Config{
		Controller: runtimeconfig.ControllerConfig{ //nolint:exhaustruct
			TargetMin:         0.21,
			TargetMax:         0.39,
			GoalLow:           0.23,
			GoalHigh:          0.30,
			Interval:          time.Minute,
			RelaxedInterval:   6 * time.Hour,
			SuppressThreshold: 0.85,
			SuppressResume:    0.70,
		},
		Estimator: runtimeconfig.EstimatorConfig{Interval: 2 * time.Second},
		Pool: runtimeconfig.PoolConfig{
			Workers:         4,
			Quantum:         50 * time.Millisecond,
			PauseThreshold:  0.85,
			ResumeThreshold: 0.70,
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
	requireLogFieldString(t, entry, "httpBind", "127.0.0.1:9000")
}

func TestLogMetadataResolutionOnline(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)

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

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)

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

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)

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

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)

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

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	cfg := runtimeconfig.Config{ //nolint:exhaustruct
		Pool: runtimeconfig.PoolConfig{
			Workers:         2,
			Quantum:         25 * time.Millisecond,
			PauseThreshold:  0.85,
			ResumeThreshold: 0.70,
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

func assertNoIMDSCalls(t *testing.T, client *stubIMDSClient) {
	t.Helper()

	if client.regionCalls != 0 || client.canonicalRegionCalls != 0 || client.instanceCalls != 0 ||
		client.compartmentCalls != 0 || client.shapeCalls != 0 {
		t.Fatalf(
			"expected offline mode to skip imds lookups, got region=%d canonical=%d instance=%d compartment=%d shape=%d",
			client.regionCalls,
			client.canonicalRegionCalls,
			client.instanceCalls,
			client.compartmentCalls,
			client.shapeCalls,
		)
	}
}

func requireOverrideIMDSLookups(t *testing.T, client *stubIMDSClient) {
	t.Helper()

	if client.regionCalls != 0 {
		t.Fatalf(
			"expected override to skip IMDS region lookup, got %d calls",
			client.regionCalls,
		)
	}

	if client.instanceCalls != 0 {
		t.Fatalf(
			"expected override to skip IMDS instance lookup, got %d calls",
			client.instanceCalls,
		)
	}

	if client.compartmentCalls != 0 {
		t.Fatalf("expected override to skip compartment lookup, got %d", client.compartmentCalls)
	}

	if client.canonicalRegionCalls != 1 {
		t.Fatalf(
			"expected canonical region lookup despite overrides, got %d",
			client.canonicalRegionCalls,
		)
	}
}

func assertOfflineLog(t *testing.T, observed *observer.ObservedLogs, expectedID string) {
	t.Helper()

	if warns := observed.FilterLevelExact(zapcore.WarnLevel).All(); len(warns) != 0 {
		t.Fatalf("expected no warnings, got %d", len(warns))
	}

	entries := observed.FilterLevelExact(zapcore.InfoLevel).All()
	if len(entries) != 1 {
		t.Fatalf("expected single info entry, got %d", len(entries))
	}

	entry := entries[0]
	if got := fieldString(entry.Context, "instanceID"); got != expectedID {
		t.Fatalf("expected trimmed override instance id, got %q", got)
	}

	if got := fieldString(entry.Context, "controllerState"); got != adapt.StateNormal.String() {
		t.Fatalf("expected controller state %q, got %q", adapt.StateNormal.String(), got)
	}

	offline, ok := fieldBool(entry.Context, "offline")
	if !ok || !offline {
		t.Fatalf("expected offline field to be true, got %v (ok=%v)", offline, ok)
	}
}
