package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/cgroup"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

var (
	errStubMetadataLookup        = errors.New("metadata lookup failed")
	errStubMetricsStartup        = errors.New("metrics startup failed")
	errUnexpectedControllerBuild = errors.New("unexpected controller construction")
)

//nolint:funlen // integration-style test exercises metadata resolution path.
func TestPrepareControllerReturnsMetadataResolutionError(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	deps := defaultRunDeps()
	deps.detectCgroup = func() (*cgroup.CPU, error) {
		return &cgroup.CPU{
			Path:   "",
			Weight: cgroup.Weight{Path: "", Value: 0, Available: false, Err: nil},
			Max: cgroup.Max{
				Path:      "",
				Quota:     0,
				Period:    0,
				Unlimited: false,
				Available: false,
				Err:       nil,
			},
		}, nil
	}
	deps.loadConfig = func(string) (runtimeconfig.Config, error) {
		cfg := runtimeconfig.Default()
		cfg.OCI.Offline = false

		return cfg, nil
	}
	deps.newLogger = func(string) (*zap.Logger, error) {
		return logger, nil
	}
	deps.newIMDS = func() imds.Client {
		return &stubIMDSClient{compartmentErr: errStubMetadataLookup} //nolint:exhaustruct
	}
	deps.newController = func(
		context.Context,
		string,
		runtimeconfig.Config,
		imds.Client,
		adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error) {
		t.Fatal("unexpected controller construction")

		return nil, nil, errUnexpectedControllerBuild
	}

	application := newApp(deps)

	ctx, boot, exitCode, ready := application.bootstrap(
		t.Context(),
		[]string{"--mode", "enforce"},
		io.Discard,
	)
	if !ready {
		t.Fatalf("expected bootstrap to succeed, got exit code %d", exitCode)
	}
	defer boot.cleanup()

	_, exitCode, controllerReady := application.prepareController(ctx, boot)
	if controllerReady {
		t.Fatal("expected controller preparation to fail when metadata resolution errors")
	}

	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected runtime error exit code, got %d", exitCode)
	}

	entries := observed.FilterMessage("failed to resolve oci metadata").All()
	if len(entries) == 0 {
		t.Fatalf("expected metadata resolution error log, got %+v", observed.All())
	}
}

//nolint:funlen // integration-style test exercises metadata resolution path.
func TestPrepareControllerUsesIMDSMetadataWhenOverridesMissing(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	deps.detectCgroup = func() (*cgroup.CPU, error) {
		return &cgroup.CPU{
			Path:   "",
			Weight: cgroup.Weight{Path: "", Value: 0, Available: false, Err: nil},
			Max: cgroup.Max{
				Path:      "",
				Quota:     0,
				Period:    0,
				Unlimited: false,
				Available: false,
				Err:       nil,
			},
		}, nil
	}

	imdsClient := newLoggingStubIMDS(
		stubRegion,
		nil,
		stubCanonicalRegion,
		nil,
		"ocid1.instance.oc1..imds",
		nil,
		stubCompartmentID,
		nil,
		stubShapeConfig(0, 0),
		nil,
	)

	deps.loadConfig = func(string) (runtimeconfig.Config, error) {
		cfg := runtimeconfig.Default()
		cfg.OCI.CompartmentID = ""
		cfg.OCI.Region = ""
		cfg.OCI.InstanceID = ""
		cfg.OCI.Offline = false
		cfg.HTTP.Bind = ""
		cfg.Pool.Workers = 1

		return cfg, nil
	}
	deps.newLogger = func(string) (*zap.Logger, error) { return zap.NewNop(), nil }
	deps.newIMDS = func() imds.Client { return imdsClient }
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		return func(context.Context) {}, nil
	}

	application := newApp(deps)

	ctx := contextWithAssertingMetricsFactory(
		t,
		newStubMetricsClient(),
		stubCompartmentID,
		stubCanonicalRegion,
	)

	ctx, boot, exitCode, ready := application.bootstrap(
		ctx,
		[]string{"--mode", "enforce"},
		io.Discard,
	)
	if !ready {
		t.Fatalf("expected bootstrap to succeed, got exit code %d", exitCode)
	}
	defer boot.cleanup()

	runtime, exitCode, controllerReady := application.prepareController(ctx, boot)
	if !controllerReady {
		t.Fatalf("expected controller preparation to succeed, got exit code %d", exitCode)
	}
	defer runtime.cleanup(ctx)

	if runtime.cfg.OCI.CompartmentID != stubCompartmentID {
		t.Fatalf(
			"expected compartment %q, got %q",
			stubCompartmentID,
			runtime.cfg.OCI.CompartmentID,
		)
	}

	if runtime.cfg.OCI.Region != stubCanonicalRegion {
		t.Fatalf(
			"expected canonical region %q, got %q",
			stubCanonicalRegion,
			runtime.cfg.OCI.Region,
		)
	}

	stub, ok := runtime.imdsClient.(*stubIMDSClient)
	if !ok {
		t.Fatalf("unexpected imds client type %T", runtime.imdsClient)
	}

	if stub.compartmentCalls != 1 {
		t.Fatalf("expected compartment lookup, got %d", stub.compartmentCalls)
	}

	if stub.regionCalls != 1 {
		t.Fatalf("expected legacy region lookup, got %d", stub.regionCalls)
	}

	if stub.canonicalRegionCalls != 1 {
		t.Fatalf("expected canonical region lookup, got %d", stub.canonicalRegionCalls)
	}
}

//nolint:funlen // integration-style test exercises override handling with IMDS available.
func TestPrepareControllerPrefersOverridesWhenIMDSAvailable(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	deps.detectCgroup = func() (*cgroup.CPU, error) {
		return &cgroup.CPU{
			Path:   "",
			Weight: cgroup.Weight{Path: "", Value: 0, Available: false, Err: nil},
			Max: cgroup.Max{
				Path:      "",
				Quota:     0,
				Period:    0,
				Unlimited: false,
				Available: false,
				Err:       nil,
			},
		}, nil
	}

	imdsClient := newLoggingStubIMDS(
		stubRegion,
		nil,
		stubCanonicalRegion,
		nil,
		"ocid1.instance.oc1..imds",
		nil,
		stubCompartmentID,
		nil,
		stubShapeConfig(0, 0),
		nil,
	)

	deps.loadConfig = func(string) (runtimeconfig.Config, error) {
		cfg := runtimeconfig.Default()
		cfg.OCI.CompartmentID = "  " + testCompartmentOverride + "  "
		cfg.OCI.Region = "  " + overrideRegion + "  "
		cfg.OCI.InstanceID = "ocid1.instance.oc1..override"
		cfg.OCI.Offline = false
		cfg.HTTP.Bind = ""
		cfg.Pool.Workers = 1

		return cfg, nil
	}
	deps.newLogger = func(string) (*zap.Logger, error) { return zap.NewNop(), nil }
	deps.newIMDS = func() imds.Client { return imdsClient }
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		return func(context.Context) {}, nil
	}

	application := newApp(deps)

	ctx := contextWithAssertingMetricsFactory(
		t,
		newStubMetricsClient(),
		testCompartmentOverride,
		overrideRegion,
	)

	ctx, boot, exitCode, ready := application.bootstrap(
		ctx,
		[]string{"--mode", "enforce"},
		io.Discard,
	)
	if !ready {
		t.Fatalf("expected bootstrap to succeed, got exit code %d", exitCode)
	}
	defer boot.cleanup()

	runtime, exitCode, controllerReady := application.prepareController(ctx, boot)
	if !controllerReady {
		t.Fatalf("expected controller preparation to succeed, got exit code %d", exitCode)
	}
	defer runtime.cleanup(ctx)

	if runtime.cfg.OCI.CompartmentID != testCompartmentOverride {
		t.Fatalf(
			"expected compartment override %q, got %q",
			testCompartmentOverride,
			runtime.cfg.OCI.CompartmentID,
		)
	}

	if runtime.cfg.OCI.Region != overrideRegion {
		t.Fatalf("expected region override %q, got %q", overrideRegion, runtime.cfg.OCI.Region)
	}

	stub, ok := runtime.imdsClient.(*stubIMDSClient)
	if !ok {
		t.Fatalf("unexpected imds client type %T", runtime.imdsClient)
	}

	if stub.compartmentCalls != 0 {
		t.Fatalf("expected overrides to skip compartment lookup, got %d", stub.compartmentCalls)
	}

	if stub.regionCalls != 0 {
		t.Fatalf("expected overrides to skip region lookup, got %d", stub.regionCalls)
	}

	if stub.canonicalRegionCalls != 0 {
		t.Fatalf(
			"expected overrides to skip canonical region lookup, got %d",
			stub.canonicalRegionCalls,
		)
	}
}

//nolint:funlen // integration-style test exercises sequential wiring paths
func TestPrepareControllerReturnsMetricsStartupError(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	deps := defaultRunDeps()
	deps.detectCgroup = func() (*cgroup.CPU, error) {
		return &cgroup.CPU{
			Path:   "",
			Weight: cgroup.Weight{Path: "", Value: 0, Available: false, Err: nil},
			Max: cgroup.Max{
				Path:      "",
				Quota:     0,
				Period:    0,
				Unlimited: false,
				Available: false,
				Err:       nil,
			},
		}, nil
	}
	deps.loadConfig = func(string) (runtimeconfig.Config, error) {
		cfg := runtimeconfig.Default()
		cfg.HTTP.Bind = "127.0.0.1:9000"
		cfg.OCI.Offline = true

		return cfg, nil
	}
	deps.newLogger = func(string) (*zap.Logger, error) {
		return logger, nil
	}
	deps.newIMDS = func() imds.Client { return &stubIMDSClient{} } //nolint:exhaustruct

	var ctrl stubController

	pool := new(stubPoolStarter)
	deps.newController = func(
		context.Context,
		string,
		runtimeconfig.Config,
		imds.Client,
		adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error) {
		ctrl.mode = modeDryRun

		return &ctrl, pool, nil
	}
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		return nil, errStubMetricsStartup
	}

	application := newApp(deps)

	ctx, boot, exitCode, ready := application.bootstrap(
		t.Context(),
		[]string{"--mode", "dry-run"},
		io.Discard,
	)
	if !ready {
		t.Fatalf("expected bootstrap to succeed, got exit code %d", exitCode)
	}
	defer boot.cleanup()

	_, exitCode, controllerReady := application.prepareController(ctx, boot)
	if controllerReady {
		t.Fatal("expected controller preparation to fail when metrics startup fails")
	}

	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected runtime error exit code, got %d", exitCode)
	}

	entries := observed.FilterMessage("failed to start metrics server").All()
	if len(entries) == 0 {
		t.Fatalf("expected metrics startup error log, got %+v", observed.All())
	}

	if ctrl.runCalled {
		t.Fatal("expected controller Run to be skipped when metrics startup fails")
	}
}

//nolint:funlen // integration-style test exercises error wiring and logging.
func TestPrepareControllerHandlesConstructionFailure(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	deps := defaultRunDeps()
	deps.detectCgroup = func() (*cgroup.CPU, error) {
		return &cgroup.CPU{
			Path:   "",
			Weight: cgroup.Weight{Path: "", Value: 0, Available: false, Err: nil},
			Max: cgroup.Max{
				Path:      "",
				Quota:     0,
				Period:    0,
				Unlimited: false,
				Available: false,
				Err:       nil,
			},
		}, nil
	}
	deps.loadConfig = func(string) (runtimeconfig.Config, error) {
		cfg := runtimeconfig.Default()
		cfg.OCI.Offline = true

		return cfg, nil
	}
	deps.newLogger = func(string) (*zap.Logger, error) {
		return logger, nil
	}
	deps.newIMDS = func() imds.Client { return &stubIMDSClient{} } //nolint:exhaustruct
	deps.newController = func(
		context.Context,
		string,
		runtimeconfig.Config,
		imds.Client,
		adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error) {
		return nil, nil, adapt.ErrInvalidConfig
	}

	application := newApp(deps)

	ctx, boot, exitCode, ready := application.bootstrap(
		t.Context(),
		[]string{"--mode", "enforce"},
		io.Discard,
	)
	if !ready {
		t.Fatalf("expected bootstrap to succeed, got exit code %d", exitCode)
	}
	defer boot.cleanup()

	_, exitCode, controllerReady := application.prepareController(ctx, boot)
	if controllerReady {
		t.Fatal("expected controller preparation to fail when controller factory errors")
	}

	if exitCode != exitCodeParseError {
		t.Fatalf("expected parse error exit code, got %d", exitCode)
	}

	entries := observed.FilterMessage("failed to build controller").All()
	if len(entries) == 0 {
		t.Fatalf("expected controller build error log, got %+v", observed.All())
	}
}
