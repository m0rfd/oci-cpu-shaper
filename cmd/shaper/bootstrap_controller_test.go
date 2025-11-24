package main

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/adapt"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

func TestStartWorkerPoolStartsPool(t *testing.T) {
	t.Parallel()

	pool := &stubPoolStarter{startCount: 0, workers: 2, quantum: time.Second}
	startWorkerPool(context.Background(), zap.NewNop(), pool)

	if pool.startCount != 1 {
		t.Fatalf("expected worker pool to start once, got %d", pool.startCount)
	}
}

func TestStartControllerRunsController(t *testing.T) {
	t.Parallel()

	ctrl := &stubController{
		mode:        modeEnforce,
		runErr:      nil,
		runCalled:   false,
		deadline:    time.Time{},
		deadlineSet: false,
		state:       adapt.StateNormal,
		lastErr:     nil,
		estErr:      nil,
	}
	pool := &stubPoolStarter{startCount: 0, workers: 1, quantum: time.Millisecond}

	cfg := runtimeconfig.Default()
	cfg.OCI.Offline = true

	runtime := controllerRuntime{
		cfg: cfg,
		opts: options{
			configPath:    defaultConfigPath,
			logLevel:      defaultLogLevel,
			mode:          modeEnforce,
			shutdownAfter: 0,
			showVersion:   false,
		},
		logger:          zap.NewNop(),
		imdsClient:      nil,
		metricsExporter: nil,
		metricsShutdown: nil,
		metricsCancel:   nil,
		controller:      ctrl,
		pool:            pool,
	}

	exitCode := startController(context.Background(), runtime)
	if exitCode != exitCodeSuccess {
		t.Fatalf("expected controller to report success, got %d", exitCode)
	}

	if !ctrl.runCalled {
		t.Fatal("expected controller run to execute")
	}

	if pool.startCount != 1 {
		t.Fatalf("expected worker pool to start once, got %d", pool.startCount)
	}
}
