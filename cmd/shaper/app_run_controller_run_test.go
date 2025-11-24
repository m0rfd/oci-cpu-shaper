package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"oci-cpu-shaper/internal/buildinfo"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

var errStubLoggerBoom = errors.New("logger failure")

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

	application := newApp(deps)

	args := []string{"--mode", "enforce", "--log-level", "debug"}

	ctx, boot, exitCode, ready := application.bootstrap(
		t.Context(),
		args,
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

	exitCode = runtime.start(ctx)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}

	requireRunInvoked(t, &ctrl)

	if ctrl.mode != modeEnforce {
		t.Fatalf("expected controller mode \"enforce\", got %q", ctrl.mode)
	}

	if pool.startCount != 1 {
		t.Fatalf("expected pool Start to be called once, got %d", pool.startCount)
	}

	assertInfoLogEntry(t, observed.All(), "test-version", "test-commit", "2024-05-01")
}

func TestBootstrapReturnsLoggerConfigurationError(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	deps := defaultRunDeps()
	deps.currentBuildInfo = func() buildinfo.Info {
		return stubBuildInfo("", "", "")
	}
	deps.newLogger = func(string) (*zap.Logger, error) {
		return nil, errStubLoggerBoom
	}

	application := newApp(deps)

	_, _, exitCode, ready := application.bootstrap(t.Context(), nil, &stderr)
	if ready {
		t.Fatal("expected bootstrap to fail when logger configuration fails")
	}

	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected exit code 1 when logger configuration fails, got %d", exitCode)
	}

	if got := stderr.String(); !strings.Contains(got, "failed to configure logger") {
		t.Fatalf("expected logger configuration failure message, got %q", got)
	}
}

//nolint:funlen // integration-style test exercises controller run wiring.
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

	application := newApp(deps)

	args := []string{"--mode", "noop", "--log-level", "debug"}

	ctx, boot, exitCode, ready := application.bootstrap(
		t.Context(),
		args,
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

	exitCode = runtime.start(ctx)
	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected exit code 1 when controller.Run returns an error, got %d", exitCode)
	}

	if !ctrl.runCalled {
		t.Fatal("expected controller Run to be invoked")
	}

	entries := observed.FilterMessage("controller execution failed").All()
	if len(entries) == 0 {
		t.Fatalf("expected controller execution failure log entry, got %+v", observed.All())
	}
}
