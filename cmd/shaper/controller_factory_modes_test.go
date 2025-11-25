package main

import (
	"context"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/oci/metricsclient"
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

func TestDefaultControllerFactoryTrimsModeToEnforce(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = stubCompartmentID
	cfg.OCI.Region = stubRegion

	imdsClient := new(stubIMDSClient)
	imdsClient.instanceID = "ocid1.instance.oc1..dryrun"

	fakeMetrics := newStubMetricsClient()
	ctx := contextWithStubMetrics(t, fakeMetrics)

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

	if controller.Mode() != modeEnforce {
		t.Fatalf("expected default enforce mode, got %q", controller.Mode())
	}
}

func TestDefaultControllerFactoryBuildsAdaptiveController(t *testing.T) {
	t.Parallel()

	fakeMetrics := newStubMetricsClient()
	ctx := contextWithStubMetrics(t, fakeMetrics)

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

	ctx := metricsclient.WithBuilder(
		context.Background(),
		func(string, string) (metricsclient.MetricsClient, error) {
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
