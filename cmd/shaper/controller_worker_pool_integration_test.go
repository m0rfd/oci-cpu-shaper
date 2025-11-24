package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

var errWorkerStart = errors.New("worker start failed")

func TestStartControllerSkipsNilWorkerPool(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.Offline = true

	controller := &stubController{mode: modeEnforce} //nolint:exhaustruct
	logCore, observed := observer.New(zap.InfoLevel)

	runtime := controllerRuntime{ //nolint:exhaustruct
		cfg: cfg,
		opts: options{
			configPath:    defaultConfigPath,
			logLevel:      defaultLogLevel,
			mode:          modeEnforce,
			shutdownAfter: 0,
			showVersion:   false,
		},
		logger:     zap.New(logCore),
		controller: controller,
	}

	exitCode := startController(context.Background(), runtime)

	if exitCode != exitCodeSuccess {
		t.Fatalf("expected success exit code, got %d", exitCode)
	}

	if !controller.runCalled {
		t.Fatal("expected controller run to be invoked")
	}

	if entries := observed.FilterMessage("starting worker pool").All(); len(entries) != 0 {
		t.Fatalf("expected worker pool startup log to be skipped, got %+v", entries)
	}
}

func TestStartControllerLogsWorkerStartErrors(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.Offline = true

	pool := &trackingPoolStarter{ //nolint:exhaustruct
		workers:        2,
		quantum:        250 * time.Millisecond,
		workerStartErr: errWorkerStart,
	}

	controller := &stubController{mode: modeEnforce} //nolint:exhaustruct
	logCore, observed := observer.New(zap.InfoLevel)

	runtime := controllerRuntime{ //nolint:exhaustruct
		cfg: cfg,
		opts: options{
			configPath:    defaultConfigPath,
			logLevel:      defaultLogLevel,
			mode:          modeEnforce,
			shutdownAfter: 0,
			showVersion:   false,
		},
		logger:     zap.New(logCore),
		controller: controller,
		pool:       pool,
	}

	exitCode := startController(context.Background(), runtime)

	assertControllerRunSuccess(t, exitCode, controller)
	assertWorkerPoolTracking(t, pool)
	assertWorkerStartWarning(t, observed, errWorkerStart)
}

func assertControllerRunSuccess(t *testing.T, exitCode int, controller *stubController) {
	t.Helper()

	if exitCode != exitCodeSuccess {
		t.Fatalf("expected success exit code, got %d", exitCode)
	}

	if controller == nil || !controller.runCalled {
		t.Fatal("expected controller run to be invoked")
	}
}

func assertWorkerPoolTracking(t *testing.T, pool *trackingPoolStarter) {
	t.Helper()

	if pool == nil {
		t.Fatal("expected worker pool to be initialized")
	}

	if pool.handlerSet != 1 {
		t.Fatalf("expected worker start error handler to be set once, got %d", pool.handlerSet)
	}

	if pool.startCalls != 1 {
		t.Fatalf("expected worker pool start to be called once, got %d", pool.startCalls)
	}

	if pool.workerCalls == 0 || pool.quantumCalls == 0 {
		t.Fatalf(
			"expected workers and quantum to be queried, got workers=%d quantum=%d",
			pool.workerCalls,
			pool.quantumCalls,
		)
	}
}

func assertWorkerStartWarning(t *testing.T, observed *observer.ObservedLogs, workerErr error) {
	t.Helper()

	warnings := observed.FilterMessage("worker failed to enter sched_idle").All()
	if len(warnings) != 1 {
		t.Fatalf("expected worker start warning log, got %+v", observed.All())
	}

	var loggedErr error

	for _, field := range warnings[0].Context {
		if field.Key == "error" {
			if err, ok := field.Interface.(error); ok {
				loggedErr = err
			}

			break
		}
	}

	if loggedErr == nil || !errors.Is(loggedErr, workerErr) {
		t.Fatalf(
			"expected warning to include worker error %v, got %+v",
			workerErr,
			warnings[0].Context,
		)
	}
}
