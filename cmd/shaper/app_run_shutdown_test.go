package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"oci-cpu-shaper/internal/buildinfo"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

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
