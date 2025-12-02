package main

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"oci-cpu-shaper/pkg/adapt"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

type contextTrackingPool struct {
	startCalls int
	startValue any
	valueKey   any
	workers    int
	quantum    time.Duration
	handler    func(error)
}

func (p *contextTrackingPool) Start(ctx context.Context) {
	p.startCalls++
	p.startValue = ctx.Value(p.valueKey)

	if p.handler != nil {
		p.handler(nil)
	}
}

func (p *contextTrackingPool) Workers() int {
	if p.workers <= 0 {
		return 1
	}

	return p.workers
}

func (p *contextTrackingPool) Quantum() time.Duration {
	if p.quantum <= 0 {
		return time.Millisecond
	}

	return p.quantum
}

func (p *contextTrackingPool) SetWorkerStartErrorHandler(handler func(err error)) {
	p.handler = handler
}

func TestStartWorkerPoolSkipsNilPool(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	startWorkerPool(context.Background(), logger, nil)

	if observed.Len() != 0 {
		t.Fatalf("expected no logs when pool starter is nil, got %+v", observed.All())
	}
}

func TestStartWorkerPoolIgnoresNilWorkerStartError(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	pool := &trackingPoolStarter{ //nolint:exhaustruct
		workers: 3,
		quantum: time.Millisecond,
	}

	startWorkerPool(context.Background(), logger, pool)

	if pool.handlerSet != 1 {
		t.Fatalf("expected worker start error handler to be set once, got %d", pool.handlerSet)
	}

	if pool.startCalls != 1 {
		t.Fatalf("expected worker pool to start once, got %d", pool.startCalls)
	}

	if pool.workerCalls != 1 || pool.quantumCalls != 1 {
		t.Fatalf(
			"expected worker and quantum to be read once, got workerCalls=%d quantumCalls=%d",
			pool.workerCalls,
			pool.quantumCalls,
		)
	}

	warnings := observed.FilterLevelExact(zap.WarnLevel).All()
	if len(warnings) != 0 {
		t.Fatalf("expected no warning logs for nil worker error, got %+v", warnings)
	}
}

func TestStartWorkerPoolPassesContextToStart(t *testing.T) {
	t.Parallel()

	type ctxKey struct{}

	ctx := context.WithValue(context.Background(), ctxKey{}, "provided context")
	pool := &contextTrackingPool{
		startCalls: 0,
		startValue: nil,
		valueKey:   ctxKey{},
		workers:    2,
		quantum:    2 * time.Millisecond,
		handler:    nil,
	}

	startWorkerPool(ctx, zap.NewNop(), pool)

	if pool.startCalls != 1 {
		t.Fatalf("expected worker pool to start once, got %d", pool.startCalls)
	}

	if pool.startValue != "provided context" {
		t.Fatalf("expected start context to carry provided values, got %v", pool.startValue)
	}
}

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
