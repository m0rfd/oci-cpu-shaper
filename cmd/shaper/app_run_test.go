package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"oci-cpu-shaper/internal/buildinfo"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

func assertRunVersionPrints(t *testing.T, args []string, info buildinfo.Info) {
	t.Helper()

	var stdout bytes.Buffer

	deps := defaultRunDeps()
	deps.newLogger = func(string) (*zap.Logger, error) {
		panic("newLogger should not be called when printing version")
	}
	deps.loadConfig = func(string) (runtimeconfig.Config, error) {
		panic("loadConfig should not be called when printing version")
	}
	deps.currentBuildInfo = func() buildinfo.Info {
		return info
	}
	deps.versionWriter = &stdout

	exitCode := run(t.Context(), args, deps, io.Discard)
	if exitCode != exitCodeSuccess {
		t.Fatalf("expected exit code %d, got %d", exitCodeSuccess, exitCode)
	}

	expected := fmt.Sprintf(
		"{Version:%s GitCommit:%s BuildDate:%s}\n",
		info.Version,
		info.GitCommit,
		info.BuildDate,
	)
	if stdout.String() != expected {
		t.Fatalf("expected stdout %q, got %q", expected, stdout.String())
	}
}

func TestRunVersionFlagPrintsBuildInfo(t *testing.T) {
	t.Parallel()

	info := stubBuildInfo("1.2.3", "commit-hash", "2024-06-01")

	assertRunVersionPrints(t, []string{"--version"}, info)
}

func TestRunVersionSubcommandPrintsBuildInfo(t *testing.T) {
	t.Parallel()

	info := stubBuildInfo("0.0.1", "deadbeef", "2024-07-04")

	assertRunVersionPrints(t, []string{"version"}, info)
}

//nolint:funlen // integration-style test exercises end-to-end wiring.
func TestRunSuccessfulPath(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	deps := defaultRunDeps()
	deps.currentBuildInfo = func() buildinfo.Info {
		return stubBuildInfo("test-version", "test-commit", "2024-05-01")
	}
	deps.newLogger = func(level string) (*zap.Logger, error) {
		if level != "debug" {
			t.Fatalf("expected log level \"debug\", got %q", level)
		}

		return logger, nil
	}

	var ctrl stubController

	pool := new(stubPoolStarter)

	deps.loadConfig = loadConfigStub()
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		return func(context.Context) {}, nil
	}

	deps.newController = func(
		ctx context.Context,
		mode string,
		cfg runtimeconfig.Config,
		imdsClient imds.Client,
		_ adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error) {
		_ = ctx
		_ = cfg
		_ = imdsClient
		ctrl.mode = mode

		return &ctrl, pool, nil
	}

	exitCode := run(
		t.Context(),
		[]string{"--mode", "enforce", "--log-level", "debug"},
		deps,
		io.Discard,
	)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}

	if !ctrl.runCalled {
		t.Fatal("expected controller Run to be called")
	}

	if ctrl.mode != modeEnforce {
		t.Fatalf("expected controller mode \"enforce\", got %q", ctrl.mode)
	}

	if pool.startCount != 1 {
		t.Fatalf("expected pool Start to be called once, got %d", pool.startCount)
	}

	assertInfoLogEntry(t, observed.All(), "test-version", "test-commit", "2024-05-01")
}

func TestRunAppliesShutdownAfter(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.DebugLevel)
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
	deps.loadConfig = loadConfigStub()
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		return func(context.Context) {}, nil
	}

	ctrl := new(stubController)

	deps.newController = func(
		ctx context.Context,
		mode string,
		cfg runtimeconfig.Config,
		imdsClient imds.Client,
		_ adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error) {
		_ = cfg
		_ = imdsClient

		controllerCtxDeadline, controllerCtxHasDeadline := ctx.Deadline()
		if !controllerCtxHasDeadline {
			t.Fatal("expected controller factory context to include deadline")
		}

		if time.Until(controllerCtxDeadline) <= 0 {
			t.Fatal("expected controller deadline to be in the future when factory executed")
		}

		ctrl.mode = mode

		return ctrl, nil, nil
	}

	exitCode := run(t.Context(), []string{"--shutdown-after", "200ms"}, deps, io.Discard)
	if exitCode != exitCodeSuccess {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}

	requireRunInvoked(t, ctrl)
	requireDeadlineCaptured(t, ctrl)

	entries := observed.FilterMessage("starting oci-cpu-shaper").All()
	requireShutdownDuration(t, entries, 200*time.Millisecond)
}

func TestRunHandlesContextShutdown(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		runErr error
		reason string
	}{
		{
			name:   "deadline exceeded",
			runErr: fmt.Errorf("adaptive controller run: %w", context.DeadlineExceeded),
			reason: context.DeadlineExceeded.Error(),
		},
		{
			name:   "context canceled",
			runErr: fmt.Errorf("adaptive controller run: %w", context.Canceled),
			reason: context.Canceled.Error(),
		},
	}

	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			runShutdownScenario(t, scenario.runErr, scenario.reason)
		})
	}
}

func TestRunReturnsParseErrorExitCode(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	deps := defaultRunDeps()
	deps.currentBuildInfo = func() buildinfo.Info {
		return stubBuildInfo("", "", "")
	}

	exitCode := run(t.Context(), []string{"--mode", "invalid"}, deps, &stderr)
	if exitCode != exitCodeParseError {
		t.Fatalf("expected exit code 2 for parse errors, got %d", exitCode)
	}

	if got := stderr.String(); !strings.Contains(got, "unsupported mode") {
		t.Fatalf("expected error message about unsupported mode, got %q", got)
	}
}

func TestRunReturnsLoggerConfigurationError(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	deps := defaultRunDeps()
	deps.currentBuildInfo = func() buildinfo.Info {
		return stubBuildInfo("", "", "")
	}
	deps.newLogger = func(string) (*zap.Logger, error) {
		return nil, errStubLoggerBoom
	}

	exitCode := run(t.Context(), nil, deps, &stderr)
	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected exit code 1 when logger configuration fails, got %d", exitCode)
	}

	if got := stderr.String(); !strings.Contains(got, "failed to configure logger") {
		t.Fatalf("expected logger configuration failure message, got %q", got)
	}
}

func TestRunHandlesControllerError(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	var ctrl stubController

	ctrl.runErr = errStubControllerRun

	deps := defaultRunDeps()
	deps.currentBuildInfo = func() buildinfo.Info {
		return stubBuildInfo("test-version", "", "")
	}
	deps.newLogger = func(string) (*zap.Logger, error) {
		return logger, nil
	}

	deps.loadConfig = loadConfigStub()
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		return func(context.Context) {}, nil
	}

	deps.newController = func(
		ctx context.Context,
		mode string,
		cfg runtimeconfig.Config,
		imdsClient imds.Client,
		_ adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error) {
		_ = ctx
		_ = cfg
		_ = imdsClient
		ctrl.mode = mode

		return &ctrl, nil, nil
	}

	exitCode := run(
		t.Context(),
		[]string{"--mode", "noop", "--log-level", "debug"},
		deps,
		io.Discard,
	)
	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected exit code 1 when controller.Run returns an error, got %d", exitCode)
	}

	if !ctrl.runCalled {
		t.Fatal("expected controller Run to be invoked")
	}

	failureEntries := observed.FilterMessage("controller execution failed").All()
	if len(failureEntries) == 0 {
		t.Fatalf("expected controller failure log, got %+v", observed.All())
	}
}

func TestRunHandlesControllerFactoryError(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	deps.currentBuildInfo = func() buildinfo.Info {
		return stubBuildInfo("test-version", "", "")
	}
	deps.newLogger = func(string) (*zap.Logger, error) {
		return zap.NewNop(), nil
	}
	deps.loadConfig = func(string) (runtimeconfig.Config, error) {
		cfg := runtimeconfig.Default()
		cfg.OCI.CompartmentID = stubCompartmentID

		return cfg, nil
	}
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		return func(context.Context) {}, nil
	}
	deps.newController = func(
		context.Context,
		string,
		runtimeconfig.Config,
		imds.Client,
		adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error) {
		return nil, nil, errStubControllerRun
	}

	exitCode := run(t.Context(), []string{"--mode", "enforce"}, deps, io.Discard)
	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected runtime error exit code, got %d", exitCode)
	}
}

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

//nolint:funlen // integration-style test exercises metrics wiring end to end.
func TestRunExposesMetricsOffline(t *testing.T) {
	t.Parallel()

	serverCh := make(chan *httptest.Server, 1)
	deps := newOfflineRunDeps(t, serverCh)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	exitCh := make(chan int, 1)

	go func() {
		exitCh <- run(ctx, []string{"--mode", "dry-run"}, deps, io.Discard)
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
			"shaper_mode{mode=\"dry-run\"} 1",
			"shaper_enforcing 0",
			"shaper_state{state=\"normal\"} 1",
			"shaper_target_ratio 0.330000",
			"worker_count 4",
			"duty_cycle_ms 2.000",
			"host_cpu_percent 50.00",
			"oci_p95 0.280000",
			"oci_last_success_epoch 1700000100",
		},
	)

	snapshot := fetchHealthSnapshot(ctx, t, server.Client(), server.URL)

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

func expectMetricsSnippets(t *testing.T, output string, snippets []string) {
	t.Helper()

	for _, snippet := range snippets {
		if !strings.Contains(output, snippet) {
			t.Fatalf("expected metrics output to contain %q, got:\n%s", snippet, output)
		}
	}
}

type healthSnapshot struct {
	State          string `json:"state"`
	LastOCIError   string `json:"ociError"`
	EstimatorError string `json:"estimatorError"`
}

func fetchHealthSnapshot(
	ctx context.Context,
	t *testing.T,
	client *http.Client,
	baseURL string,
) healthSnapshot {
	t.Helper()

	request, buildErr := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
	if buildErr != nil {
		t.Fatalf("build health request: %v", buildErr)
	}

	response, doErr := client.Do(request)
	if doErr != nil {
		t.Fatalf("GET /healthz failed: %v", doErr)
	}

	defer func() {
		closeErr := response.Body.Close()
		if closeErr != nil {
			t.Fatalf("close health response body: %v", closeErr)
		}
	}()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /healthz, got %d", response.StatusCode)
	}

	decoder := json.NewDecoder(response.Body)

	var snapshot healthSnapshot

	decodeErr := decoder.Decode(&snapshot)
	if decodeErr != nil {
		t.Fatalf("decode health response: %v", decodeErr)
	}

	return snapshot
}

func assertInfoLogEntry(
	t *testing.T,
	entries []observer.LoggedEntry,
	version, commit, date string,
) {
	t.Helper()

	var infoEntry *observer.LoggedEntry

	for i := range entries {
		if entries[i].Message == "starting oci-cpu-shaper" {
			infoEntry = &entries[i]

			break
		}
	}

	if infoEntry == nil {
		t.Fatalf("expected info log entry, got %+v", entries)
	}

	if got := fieldString(infoEntry.Context, "version"); got != version {
		t.Fatalf("expected version field %q, got %q", version, got)
	}

	if got := fieldString(infoEntry.Context, "commit"); got != commit {
		t.Fatalf("expected commit field %q, got %q", commit, got)
	}

	if got := fieldString(infoEntry.Context, "buildDate"); got != date {
		t.Fatalf("expected buildDate field %q, got %q", date, got)
	}
}

func requireRunInvoked(t *testing.T, ctrl *stubController) {
	t.Helper()

	if ctrl == nil || !ctrl.runCalled {
		t.Fatal("expected controller Run to be invoked")
	}
}

func requireDeadlineCaptured(t *testing.T, ctrl *stubController) {
	t.Helper()

	if ctrl == nil {
		t.Fatal("controller stub is nil")
	}

	if !ctrl.deadlineSet {
		t.Fatal("expected controller Run to capture deadline")
	}

	if ctrl.deadline.IsZero() {
		t.Fatal("expected controller Run deadline to be set")
	}
}

func requireShutdownDuration(
	t *testing.T,
	entries []observer.LoggedEntry,
	expected time.Duration,
) {
	t.Helper()

	if len(entries) == 0 {
		t.Fatalf("expected startup log entry, got %+v", entries)
	}

	duration, ok := fieldDuration(entries[0].Context, "shutdownAfter")
	if !ok || duration != expected {
		t.Fatalf("expected shutdownAfter duration %v, got %v (present=%v)", expected, duration, ok)
	}
}

func runShutdownScenario(t *testing.T, runErr error, reason string) {
	t.Helper()

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	ctrl := new(stubController)
	ctrl.runErr = runErr

	deps := defaultRunDeps()
	deps.currentBuildInfo = func() buildinfo.Info {
		return stubBuildInfo("test-version", "", "")
	}
	deps.newLogger = func(string) (*zap.Logger, error) {
		return logger, nil
	}
	deps.loadConfig = loadConfigStub()
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		return func(context.Context) {}, nil
	}
	deps.newController = func(
		context.Context,
		string,
		runtimeconfig.Config,
		imds.Client,
		adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error) {
		return ctrl, nil, nil
	}

	exitCode := run(t.Context(), []string{"--shutdown-after", "50ms"}, deps, io.Discard)
	if exitCode != exitCodeSuccess {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}

	requireRunInvoked(t, ctrl)

	stoppedEntries := observed.FilterMessage("controller stopped").All()
	if len(stoppedEntries) != 1 {
		t.Fatalf("expected controller stopped log entry, got %+v", observed.All())
	}

	if got := fieldString(stoppedEntries[0].Context, "reason"); got != reason {
		t.Fatalf("expected reason %q, got %q", reason, got)
	}

	if failureEntries := observed.FilterMessage("controller execution failed").All(); len(
		failureEntries,
	) != 0 {
		t.Fatalf("expected no failure logs, got %+v", failureEntries)
	}
}
