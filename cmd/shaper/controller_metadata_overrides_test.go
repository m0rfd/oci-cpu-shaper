package main

import (
	"context"
	"errors"
	"testing"

	"oci-cpu-shaper/pkg/oci"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

func TestBuildAdaptiveControllerUsesConfiguredInstanceID(t *testing.T) {
	t.Parallel()

	stubMetrics := newStubMetricsClient()
	ctx := contextWithAssertingMetricsFactory(t, stubMetrics, testCompartmentOverride, stubRegion)

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

	ctx := contextWithStubMetrics(t, newStubMetricsClient())

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

	ctx := contextWithStubMetrics(t, newStubMetricsClient())

	_, _, err := buildAdaptiveController(ctx, modeEnforce, cfg, new(stubIMDSClient), nil)
	if !errors.Is(err, errControllerRegionRequired) {
		t.Fatalf("expected errControllerRegionRequired, got %v", err)
	}
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
