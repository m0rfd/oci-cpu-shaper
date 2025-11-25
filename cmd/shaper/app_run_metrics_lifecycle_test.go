package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"oci-cpu-shaper/internal/buildinfo"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRunReturnsRuntimeErrorWhenMetricsServerFails(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	deps := defaultRunDeps()
	deps.currentBuildInfo = func() buildinfo.Info {
		return stubBuildInfo("test-version", "test-commit", "2024-05-01")
	}
	deps.newLogger = func(level string) (*zap.Logger, error) {
		if level != defaultLogLevel {
			t.Fatalf("expected default log level %q, got %q", defaultLogLevel, level)
		}

		return logger, nil
	}
	deps.newIMDS = func() imds.Client {
		return newOfflineStubIMDS()
	}
	deps.loadConfig = loadConfigStub()

	ctrl := new(stubController)
	deps.newController = func(
		context.Context,
		string,
		runtimeconfig.Config,
		imds.Client,
		adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error) {
		return ctrl, nil, nil
	}
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		return nil, errMetricsServerBoom
	}

	exitCode := run(t.Context(), nil, deps, io.Discard)
	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected runtime error exit code, got %d", exitCode)
	}

	if ctrl.runCalled {
		t.Fatal("expected controller Run not to be called when metrics server fails")
	}

	entries := observed.FilterMessage("failed to start metrics server").All()
	if len(entries) != 1 {
		t.Fatalf("expected metrics server failure log entry, got %+v", observed.All())
	}
}

//nolint:funlen // coverage-focused test exercises multiple failure branches
func TestRunReturnsRuntimeErrorWhenMetadataResolutionFails(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	deps := defaultRunDeps()
	deps.currentBuildInfo = func() buildinfo.Info {
		return stubBuildInfo("test-version", "test-commit", "2024-05-01")
	}
	deps.newLogger = func(level string) (*zap.Logger, error) {
		if level != defaultLogLevel {
			t.Fatalf("expected default log level %q, got %q", defaultLogLevel, level)
		}

		return logger, nil
	}
	deps.loadConfig = func(string) (runtimeconfig.Config, error) {
		cfg := runtimeconfig.Default()
		cfg.OCI.CompartmentID = ""
		cfg.OCI.Region = ""

		return cfg, nil
	}
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		return func(context.Context) {}, nil
	}

	failingIMDS := newLoggingStubIMDS(
		"",
		errStubQueryFailure,
		"",
		errStubQueryFailure,
		"",
		nil,
		"",
		errStubQueryFailure,
		stubShapeConfig(0, 0),
		nil,
	)
	deps.newIMDS = func() imds.Client {
		return failingIMDS
	}

	controllerCalled := false
	deps.newController = func(
		context.Context,
		string,
		runtimeconfig.Config,
		imds.Client,
		adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error) {
		controllerCalled = true

		return new(stubController), nil, nil
	}

	exitCode := run(t.Context(), nil, deps, io.Discard)
	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected runtime error exit code, got %d", exitCode)
	}

	if controllerCalled {
		t.Fatal("expected controller factory not to be invoked when metadata resolution fails")
	}

	entries := observed.FilterMessage("failed to resolve oci metadata").All()
	if len(entries) != 1 {
		t.Fatalf("expected metadata failure log entry, got %+v", observed.All())
	}
}

//nolint:funlen,cyclop // integration-style test exercises metrics wiring end to end.
func TestRunExposesMetricsOffline(t *testing.T) {
	t.Parallel()

	serverCh := make(chan *httptest.Server, 1)
	deps := newOfflineRunDeps(t, serverCh)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	exitCh := make(chan int, 1)

	go func() {
		exitCh <- run(ctx, nil, deps, io.Discard)
	}()

	var server *httptest.Server
	select {
	case server = <-serverCh:
	case <-time.After(metricsServerWait):
		t.Fatal("timed out waiting for metrics server")
	}

	defer server.Close()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/metrics", nil)
	if err != nil {
		t.Fatalf("build metrics request: %v", err)
	}

	resp, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("GET /metrics failed: %v", err)
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if err != nil {
		t.Fatalf("read metrics response: %v", err)
	}

	output := string(body)
	expectMetricsSnippets(
		t,
		output,
		[]string{
			"shaper_enforcement_mode{mode=\"enforce\"} 1",
			"shaper_enforcing 1",
			"shaper_mode{state=\"normal\"} 1",
			"shaper_target_ratio 0.330000",
			"worker_count 4",
			"duty_cycle_ms 2.000",
			"host_cpu_percent 50.00",
			"oci_p95 0.280000",
			"oci_last_success_epoch 1700000100",
		},
	)

	snapshot := fetchHealthSnapshot(ctx, t, server.Client(), server.URL)

	if snapshot.Mode != modeEnforce {
		t.Fatalf("expected health mode %q, got %q", modeEnforce, snapshot.Mode)
	}

	if snapshot.State != adapt.StateNormal.String() {
		t.Fatalf("expected health state %q, got %q", adapt.StateNormal.String(), snapshot.State)
	}

	expectedOCI := errStubHealthOCI.Error()
	if snapshot.LastOCIError != expectedOCI {
		t.Fatalf("expected health OCI error %q, got %q", expectedOCI, snapshot.LastOCIError)
	}

	expectedEstimator := errStubHealthEstimator.Error()
	if snapshot.EstimatorError != expectedEstimator {
		t.Fatalf(
			"expected health estimator error %q, got %q",
			expectedEstimator,
			snapshot.EstimatorError,
		)
	}

	cancel()

	exitCode := <-exitCh
	if exitCode != exitCodeSuccess {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}
}

var (
	errStubHealthOCI       = errors.New("oci metrics unavailable")
	errStubHealthEstimator = errors.New("estimator stalled")
)

//nolint:funlen // helper configures run dependencies and keeps test setup readable.
func newOfflineRunDeps(t *testing.T, serverCh chan<- *httptest.Server) runDeps {
	t.Helper()

	deps := defaultRunDeps()
	deps.newLogger = func(string) (*zap.Logger, error) {
		return zap.NewNop(), nil
	}
	deps.loadConfig = func(string) (runtimeconfig.Config, error) {
		cfg := runtimeconfig.Default()
		cfg.OCI.Offline = true
		cfg.OCI.CompartmentID = ""
		cfg.OCI.Region = ""
		cfg.Controller.TargetStart = 0.33
		cfg.Pool.Workers = 4
		cfg.Pool.Quantum = 2 * time.Millisecond
		cfg.HTTP.Bind = ":0"

		return cfg, nil
	}
	deps.startMetricsServer = func(
		ctx context.Context,
		_ *zap.Logger,
		_ string,
		handler http.Handler,
	) (metricsShutdownFunc, error) {
		server := httptest.NewServer(handler)

		serverCh <- server

		go func() {
			<-ctx.Done()
			server.Close()
		}()

		return func(context.Context) {
			server.Close()
		}, nil
	}
	deps.newController = func(
		ctx context.Context,
		mode string,
		cfg runtimeconfig.Config,
		imdsClient imds.Client,
		recorder adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error) {
		_ = ctx
		_ = imdsClient

		if recorder != nil {
			recorder.SetMode(mode)
			recorder.SetState(adapt.StateNormal.String())
			recorder.SetTarget(cfg.Controller.TargetStart)
			recorder.ObserveHostCPU(0.5)
			recorder.ObserveOCIP95(0.28, time.Unix(1_700_000_100, 0))
		}

		pool := new(stubPoolStarter)
		pool.workers = cfg.Pool.Workers
		pool.quantum = cfg.Pool.Quantum

		controller := &blockingController{
			mode:    mode,
			state:   adapt.StateNormal,
			lastErr: errStubHealthOCI,
			estErr:  errStubHealthEstimator,
		}

		return controller, pool, nil
	}

	return deps
}
