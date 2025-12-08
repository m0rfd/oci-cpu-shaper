package main

import (
	"context"
	"errors"
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

func TestDefaultControllerFactoryNormalizesBlankModeToEnforce(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = stubCompartmentID
	cfg.OCI.Region = stubRegion

	imdsClient := new(stubIMDSClient)
	imdsClient.instanceID = "ocid1.instance.oc1..blank"

	fakeMetrics := newStubMetricsClient()
	ctx := contextWithStubMetrics(t, fakeMetrics)

	controller, pool, err := defaultControllerFactory(
		ctx,
		"",
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
		t.Fatalf("expected blank mode to normalize to enforce, got %q", controller.Mode())
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

func TestDefaultControllerFactoryErrorsWithoutIMDSForNonNoopMode(t *testing.T) {
	t.Parallel()

	controller, pool, err := defaultControllerFactory(
		context.Background(),
		modeEnforce,
		runtimeconfig.Default(),
		nil,
		nil,
	)
	if !errors.Is(err, errControllerIMDSRequired) {
		t.Fatalf("expected errControllerIMDSRequired, got %v", err)
	}

	if controller != nil {
		t.Fatalf("expected nil controller on IMDS requirement failure, got %T", controller)
	}

	if pool != nil {
		t.Fatalf("expected nil pool on IMDS requirement failure, got %#v", pool)
	}
}

func TestDefaultControllerFactoryNoopConfiguresRecorderHooks(t *testing.T) {
	t.Parallel()

	recorder := newTrackingRecorder()

	controller, pool, err := defaultControllerFactory(
		context.Background(),
		modeNoop,
		runtimeconfig.Default(),
		nil,
		recorder,
	)
	if err != nil {
		t.Fatalf("defaultControllerFactory returned error: %v", err)
	}

	if pool != nil {
		t.Fatalf("expected no pool for noop mode, got %#v", pool)
	}

	if controller.Mode() != modeNoop {
		t.Fatalf("expected noop controller mode, got %q", controller.Mode())
	}

	if recorder.mode != modeNoop || recorder.modeCalls != 1 {
		t.Fatalf(
			"expected recorder mode set to %q once, got %q after %d calls",
			modeNoop,
			recorder.mode,
			recorder.modeCalls,
		)
	}

	if recorder.state != adapt.StateNormal.String() || recorder.stateCalls != 1 {
		t.Fatalf(
			"expected recorder state set to %q once, got %q after %d calls",
			adapt.StateNormal,
			recorder.state,
			recorder.stateCalls,
		)
	}

	if recorder.targetCalls != 1 || recorder.target != 0 {
		t.Fatalf(
			"expected recorder target set to 0 once, got %.2f after %d calls",
			recorder.target,
			recorder.targetCalls,
		)
	}
}

type trackingRecorder struct {
	mode        string
	state       string
	target      float64
	modeCalls   int
	stateCalls  int
	targetCalls int
}

func newTrackingRecorder() *trackingRecorder { return new(trackingRecorder) }

func (r *trackingRecorder) SetMode(mode string) {
	r.mode = mode
	r.modeCalls++
}

func (r *trackingRecorder) SetState(state string) {
	r.state = state
	r.stateCalls++
}

func (r *trackingRecorder) SetTarget(target float64) {
	r.target = target
	r.targetCalls++
}

func (*trackingRecorder) ObserveOCIP95(float64, time.Time) {}

func (*trackingRecorder) ObserveHostCPU(float64) {}

func (*trackingRecorder) SetInterval(time.Duration) {}

func (*trackingRecorder) SetLastError(error) {}

func (*trackingRecorder) SetRelaxedSuccesses(int) {}
