package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/oci"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

const testCompartmentOverride = "ocid1.compartment.oc1..override"

func TestDefaultControllerFactoryReturnsNoopForMode(t *testing.T) {
	t.Parallel()

	noopIMDS := new(stubIMDSClient)

	controller, pool, err := defaultControllerFactory(
		context.Background(),
		modeNoop,
		runtimeconfig.Default(),
		noopIMDS,
		nil,
	)
	if err != nil {
		t.Fatalf("defaultControllerFactory returned error: %v", err)
	}

	if pool != nil {
		t.Fatalf("expected no pool for noop mode, got %#v", pool)
	}

	if got := controller.Mode(); got != modeNoop {
		t.Fatalf("expected controller mode %q, got %q", modeNoop, got)
	}

	if _, ok := controller.(*adapt.NoopController); !ok {
		t.Fatalf("expected noop controller implementation, got %T", controller)
	}
}

func TestDefaultControllerFactoryTrimsModeToDryRun(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = stubCompartmentID
	cfg.OCI.Region = stubRegion

	imdsClient := new(stubIMDSClient)
	imdsClient.instanceID = "ocid1.instance.oc1..dryrun"

	fakeMetrics := newStubMetricsClient()
	ctx := withMetricsClientFactory(
		context.Background(),
		func(string, string) (oci.MetricsClient, error) {
			return fakeMetrics, nil
		},
	)

	controller, pool, err := defaultControllerFactory(
		ctx,
		"   ",
		cfg,
		imdsClient,
		nil,
	)
	if err != nil {
		t.Fatalf("defaultControllerFactory returned error: %v", err)
	}

	if pool == nil {
		t.Fatal("expected pool to be created for adaptive controller")
	}

	if controller.Mode() != modeDryRun {
		t.Fatalf("expected modeDryRun, got %q", controller.Mode())
	}
}

func TestDefaultControllerFactoryBuildsAdaptiveController(t *testing.T) {
	t.Parallel()

	fakeMetrics := newStubMetricsClient()
	ctx := withMetricsClientFactory(
		context.Background(),
		func(string, string) (oci.MetricsClient, error) {
			return fakeMetrics, nil
		},
	)

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = "ocid1.compartment.oc1..controller"
	cfg.OCI.Region = stubRegion
	cfg.Pool.Workers = 1
	cfg.Estimator.Interval = 500 * time.Millisecond

	imdsClient := new(stubIMDSClient)
	imdsClient.instanceID = "ocid1.instance.oc1..controller"

	controller, pool, err := defaultControllerFactory(
		ctx,
		modeEnforce,
		cfg,
		imdsClient,
		nil,
	)
	if err != nil {
		t.Fatalf("defaultControllerFactory returned error: %v", err)
	}

	if pool == nil {
		t.Fatal("expected pool to be returned for adaptive controller")
	}

	if controller.Mode() != modeEnforce {
		t.Fatalf("expected enforce mode, got %q", controller.Mode())
	}
}

func TestDefaultControllerFactoryErrorsOnMissingCompartmentID(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = ""

	imdsClient := new(stubIMDSClient)
	imdsClient.instanceID = "ocid1.instance.oc1..missing"

	_, _, err := defaultControllerFactory(
		context.Background(),
		modeDryRun,
		cfg,
		imdsClient,
		nil,
	)
	if err == nil {
		t.Fatal("expected error when compartment ID is missing")
	}
}

func TestDefaultControllerFactoryPropagatesMetricsFailure(t *testing.T) {
	t.Parallel()

	ctx := withMetricsClientFactory(
		context.Background(),
		func(string, string) (oci.MetricsClient, error) {
			return nil, errStubControllerRun
		},
	)

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = "ocid1.compartment.oc1..metrics"
	cfg.OCI.Region = stubRegion

	imdsClient := new(stubIMDSClient)
	imdsClient.instanceID = "ocid1.instance.oc1..metrics"

	_, _, err := defaultControllerFactory(
		ctx,
		modeDryRun,
		cfg,
		imdsClient,
		nil,
	)
	if err == nil {
		t.Fatal("expected error when metrics client creation fails")
	}
}

func TestDefaultControllerFactoryPropagatesIMDSError(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = "ocid1.compartment.oc1..imds"
	cfg.OCI.Region = stubRegion

	failingIMDS := new(stubIMDSClient)
	failingIMDS.instanceErr = errInstanceDown

	_, _, err := defaultControllerFactory(
		context.Background(),
		modeDryRun,
		cfg,
		failingIMDS,
		nil,
	)
	if err == nil {
		t.Fatal("expected error when instance lookup fails")
	}
}

func TestBuildAdaptiveControllerUsesConfiguredInstanceID(t *testing.T) {
	t.Parallel()

	stubMetrics := newStubMetricsClient()
	ctx := withMetricsClientFactory(
		context.Background(),
		func(compartmentID, region string) (oci.MetricsClient, error) {
			if compartmentID != testCompartmentOverride {
				t.Fatalf("unexpected compartment id: %s", compartmentID)
			}

			if region != stubRegion {
				t.Fatalf("unexpected region: %s", region)
			}

			return stubMetrics, nil
		},
	)

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = testCompartmentOverride
	cfg.OCI.Region = stubRegion
	cfg.OCI.InstanceID = "  ocid1.instance.oc1..override  "
	cfg.Pool.Workers = 1

	imdsClient := new(stubIMDSClient)
	imdsClient.instanceErr = errInstanceDown

	controller, pool, err := buildAdaptiveController(
		ctx,
		modeDryRun,
		cfg,
		imdsClient,
		nil,
	)
	if err != nil {
		t.Fatalf("buildAdaptiveController returned error: %v", err)
	}

	if pool == nil {
		t.Fatal("expected worker pool to be initialized")
	}

	if controller.Mode() != modeDryRun {
		t.Fatalf("unexpected mode: %s", controller.Mode())
	}

	if imdsClient.instanceCalls != 0 {
		t.Fatalf("expected override to skip IMDS lookup, got %d calls", imdsClient.instanceCalls)
	}
}

func TestBuildAdaptiveControllerOfflineSkipsExternalDependencies(t *testing.T) {
	t.Parallel()

	ctx := withMetricsClientFactory(
		context.Background(),
		func(string, string) (oci.MetricsClient, error) {
			t.Fatal("expected offline mode to avoid metrics factory")

			return nil, errStubControllerRun
		},
	)

	cfg := runtimeconfig.Default()
	cfg.Controller.TargetStart = 0.42
	cfg.OCI.CompartmentID = ""
	cfg.OCI.InstanceID = ""
	cfg.OCI.Offline = true

	imdsClient := new(stubIMDSClient)
	imdsClient.instanceErr = errInstanceDown

	controller, pool, err := buildAdaptiveController(ctx, modeDryRun, cfg, imdsClient, nil)
	if err != nil {
		t.Fatalf("buildAdaptiveController returned error: %v", err)
	}

	if pool == nil {
		t.Fatal("expected worker pool to be initialized")
	}

	if controller.Mode() != modeDryRun {
		t.Fatalf("unexpected mode: %s", controller.Mode())
	}
}

func TestBuildAdaptiveControllerRequiresCompartmentID(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.InstanceID = "ocid1.instance.oc1..missing-compartment"
	cfg.OCI.CompartmentID = ""
	cfg.OCI.Region = stubRegion

	ctx := withMetricsClientFactory(
		context.Background(),
		func(string, string) (oci.MetricsClient, error) {
			return newStubMetricsClient(), nil
		},
	)

	_, _, err := buildAdaptiveController(ctx, modeEnforce, cfg, new(stubIMDSClient), nil)
	if !errors.Is(err, errControllerCompartmentRequired) {
		t.Fatalf("expected errControllerCompartmentRequired, got %v", err)
	}
}

func TestBuildAdaptiveControllerRequiresRegion(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.InstanceID = "ocid1.instance.oc1..missing-region"
	cfg.OCI.CompartmentID = stubCompartmentID
	cfg.OCI.Region = ""

	ctx := withMetricsClientFactory(
		context.Background(),
		func(string, string) (oci.MetricsClient, error) {
			return newStubMetricsClient(), nil
		},
	)

	_, _, err := buildAdaptiveController(ctx, modeEnforce, cfg, new(stubIMDSClient), nil)
	if !errors.Is(err, errControllerRegionRequired) {
		t.Fatalf("expected errControllerRegionRequired, got %v", err)
	}
}

func TestHandleControllerRunResultLogsCompletion(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	code := handleControllerRunResult(logger, nil)
	if code != exitCodeSuccess {
		t.Fatalf("expected success exit code, got %d", code)
	}

	entries := observed.FilterMessage("controller stopped").All()
	if len(entries) != 1 {
		t.Fatalf("expected controller stopped log entry, got %+v", observed.All())
	}

	requireLogFieldString(t, entries[0], "reason", "completed")
}

func TestResolveCompartmentAndRegionOfflineSkipsLookups(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = "  ocid1.compartment.oc1..offline  "
	cfg.OCI.Region = "  us-phoenix-1  "
	cfg.OCI.Offline = true

	metadata, err := resolveCompartmentAndRegion(t.Context(), cfg, nil)
	if err != nil {
		t.Fatalf("resolveCompartmentAndRegion returned error: %v", err)
	}

	if metadata.CompartmentID != "ocid1.compartment.oc1..offline" {
		t.Fatalf("expected trimmed compartment id, got %q", metadata.CompartmentID)
	}

	if metadata.Region != "us-phoenix-1" {
		t.Fatalf("expected trimmed region, got %q", metadata.Region)
	}
}

func TestResolveCompartmentAndRegionRequiresIMDSOnline(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = ""
	cfg.OCI.Region = ""
	cfg.OCI.Offline = false

	_, err := resolveCompartmentAndRegion(t.Context(), cfg, nil)
	if !errors.Is(err, errControllerIMDSRequired) {
		t.Fatalf("expected errControllerIMDSRequired, got %v", err)
	}
}

func TestResolveCompartmentAndRegionFallsBackToOverrides(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = "  ocid1.compartment.oc1..override  "
	cfg.OCI.Region = "  " + overrideRegion + "  "

	client := newLoggingStubIMDS(
		"",
		errRegionDown,
		"",
		errRegionDown,
		"",
		nil,
		"",
		errInstanceDown,
		stubShapeConfig(0, 0),
		nil,
	)

	metadata, err := resolveCompartmentAndRegion(t.Context(), cfg, client)
	if err != nil {
		t.Fatalf("resolveCompartmentAndRegion returned error: %v", err)
	}

	if metadata.CompartmentID != "ocid1.compartment.oc1..override" {
		t.Fatalf("unexpected compartment id %q", metadata.CompartmentID)
	}

	if metadata.Region != overrideRegion {
		t.Fatalf("unexpected region %q", metadata.Region)
	}

	if client.compartmentCalls != 1 {
		t.Fatalf("expected compartment lookup despite overrides, got %d", client.compartmentCalls)
	}

	if client.regionCalls != 1 {
		t.Fatalf("expected region lookup despite overrides, got %d", client.regionCalls)
	}

	if client.canonicalRegionCalls != 1 {
		t.Fatalf(
			"expected canonical region lookup despite overrides, got %d",
			client.canonicalRegionCalls,
		)
	}
}

func TestResolveCompartmentAndRegionFetchesFromIMDS(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = ""
	cfg.OCI.Region = ""

	client := newLoggingStubIMDS(
		stubRegion,
		nil,
		stubRegion,
		nil,
		"",
		nil,
		stubCompartmentID,
		nil,
		stubShapeConfig(0, 0),
		nil,
	)

	metadata, err := resolveCompartmentAndRegion(t.Context(), cfg, client)
	if err != nil {
		t.Fatalf("resolveCompartmentAndRegion returned error: %v", err)
	}

	if metadata.CompartmentID != stubCompartmentID {
		t.Fatalf("expected compartment %q, got %q", stubCompartmentID, metadata.CompartmentID)
	}

	if metadata.Region != stubRegion {
		t.Fatalf("expected region %s, got %q", stubRegion, metadata.Region)
	}

	if client.compartmentCalls != 1 {
		t.Fatalf("expected single compartment lookup, got %d", client.compartmentCalls)
	}

	if client.regionCalls != 1 {
		t.Fatalf("expected single region lookup, got %d", client.regionCalls)
	}

	if client.canonicalRegionCalls != 1 {
		t.Fatalf("expected single canonical region lookup, got %d", client.canonicalRegionCalls)
	}
}

func TestResolveCompartmentAndRegionPrefersCanonicalRegion(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = ""
	cfg.OCI.Region = ""

	client := newLoggingStubIMDS(
		"phx",
		nil,
		overrideRegion,
		nil,
		"",
		nil,
		stubCompartmentID,
		nil,
		stubShapeConfig(0, 0),
		nil,
	)

	metadata, err := resolveCompartmentAndRegion(t.Context(), cfg, client)
	if err != nil {
		t.Fatalf("resolveCompartmentAndRegion returned error: %v", err)
	}

	if metadata.Region != overrideRegion {
		t.Fatalf("expected canonical region %s, got %q", overrideRegion, metadata.Region)
	}

	if client.canonicalRegionCalls != 1 {
		t.Fatalf("expected canonical lookup, got %d", client.canonicalRegionCalls)
	}

	if client.regionCalls != 1 {
		t.Fatalf("expected region lookup for logging, got %d", client.regionCalls)
	}
}

func TestResolveCompartmentAndRegionFallsBackToLegacyRegion(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = ""
	cfg.OCI.Region = ""

	client := newLoggingStubIMDS(
		stubRegion,
		nil,
		"",
		errRegionDown,
		"",
		nil,
		stubCompartmentID,
		nil,
		stubShapeConfig(0, 0),
		nil,
	)

	metadata, err := resolveCompartmentAndRegion(t.Context(), cfg, client)
	if err != nil {
		t.Fatalf("resolveCompartmentAndRegion returned error: %v", err)
	}

	if metadata.Region != stubRegion {
		t.Fatalf("expected fallback region %s, got %q", stubRegion, metadata.Region)
	}

	if client.canonicalRegionCalls != 1 {
		t.Fatalf("expected canonical lookup, got %d", client.canonicalRegionCalls)
	}

	if client.regionCalls != 1 {
		t.Fatalf("expected legacy region lookup, got %d", client.regionCalls)
	}
}

func TestResolveCompartmentAndRegionPrefersIMDSValues(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = "ocid1.compartment.oc1..override"
	cfg.OCI.Region = overrideRegion

	client := newLoggingStubIMDS(
		stubRegion,
		nil,
		stubRegion,
		nil,
		"",
		nil,
		stubCompartmentID,
		nil,
		stubShapeConfig(0, 0),
		nil,
	)

	metadata, err := resolveCompartmentAndRegion(t.Context(), cfg, client)
	if err != nil {
		t.Fatalf("resolveCompartmentAndRegion returned error: %v", err)
	}

	if metadata.CompartmentID != stubCompartmentID {
		t.Fatalf("expected compartment %q, got %q", stubCompartmentID, metadata.CompartmentID)
	}

	if metadata.Region != stubRegion {
		t.Fatalf("expected region %s, got %q", stubRegion, metadata.Region)
	}

	if client.canonicalRegionCalls != 1 {
		t.Fatalf("expected canonical region lookup, got %d", client.canonicalRegionCalls)
	}
}

func TestExitCodeForConfigError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "invalid config",
			err:  adapt.ErrInvalidConfig,
			want: exitCodeParseError,
		},
		{
			name: "runtime error",
			err:  errStubControllerRun,
			want: exitCodeRuntimeError,
		},
		{
			name: "nil error",
			err:  nil,
			want: exitCodeRuntimeError,
		},
	}

	for _, tc := range testCases {
		testCase := tc

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := exitCodeForConfigError(testCase.err); got != testCase.want {
				t.Fatalf("expected %d, got %d", testCase.want, got)
			}
		})
	}
}
