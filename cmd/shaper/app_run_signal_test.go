package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

var errRunWithoutContextError = errors.New("run finished without context error")

//nolint:paralleltest // signal handling uses global state; keep the test serialized.
func TestAppRunCancelsOnSignal(t *testing.T) {
	controller := newSignalAwareController()

	deps := setupSignalAwareDeps(t, controller)
	application := newApp(deps)

	ctx := t.Context()

	exitCh := make(chan int, 1)

	go func() {
		exitCh <- application.Run(ctx, []string{}, io.Discard)
	}()

	waitForSignal(t, controller.started, "controller did not start")

	sendInterrupt(t)

	waitForSignal(t, controller.canceled, "controller did not receive cancellation")

	exitCode := waitForExitCode(t, exitCh)

	if exitCode != exitCodeSuccess {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}

	contextErr := controller.contextErr()
	if !errors.Is(contextErr, context.Canceled) {
		t.Fatalf("expected context cancellation, got: %v", contextErr)
	}
}

func setupSignalAwareDeps(t *testing.T, controller *signalAwareController) runDeps {
	t.Helper()

	deps := defaultRunDeps()
	deps.newLogger = func(string) (*zap.Logger, error) {
		return zaptest.NewLogger(t), nil
	}
	deps.loadConfig = loadConfigStub()
	deps.newIMDS = func() imds.Client {
		return newOfflineStubIMDS()
	}
	deps.startMetricsServer = func(
		_ context.Context,
		_ *zap.Logger,
		_ string,
		_ http.Handler,
	) (metricsShutdownFunc, error) {
		return func(context.Context) {}, nil
	}
	deps.newController = func(
		_ context.Context,
		mode string,
		_ runtimeconfig.Config,
		_ imds.Client,
		_ adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error) {
		controller.mode = mode
		controller.state = adapt.StateNormal

		return controller, nil, nil
	}

	return deps
}

func sendInterrupt(t *testing.T) {
	t.Helper()

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("failed to find process: %v", err)
	}

	signalErr := proc.Signal(os.Interrupt)
	if signalErr != nil {
		t.Fatalf("failed to send interrupt: %v", signalErr)
	}
}

type signalAwareController struct {
	started  chan struct{}
	canceled chan struct{}

	mode   string
	state  adapt.State
	last   error
	estErr error

	startOnce sync.Once
	stopOnce  sync.Once

	mu     sync.Mutex
	ctxErr error
}

func newSignalAwareController() *signalAwareController {
	return &signalAwareController{ //nolint:exhaustruct
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		state:    adapt.StateNormal,
	}
}

func (c *signalAwareController) Run(ctx context.Context) error {
	c.startOnce.Do(func() {
		close(c.started)
	})

	<-ctx.Done()

	c.mu.Lock()
	c.ctxErr = ctx.Err()
	c.mu.Unlock()

	c.stopOnce.Do(func() {
		close(c.canceled)
	})

	if ctx.Err() != nil {
		return fmt.Errorf("controller run: %w", ctx.Err())
	}

	return errRunWithoutContextError
}

func (c *signalAwareController) Mode() string { return c.mode }

func (c *signalAwareController) State() adapt.State { return c.state }

func (c *signalAwareController) LastError() error { return c.last }

func (c *signalAwareController) LastEstimatorError() error { return c.estErr }

func (c *signalAwareController) contextErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ctxErr
}

func waitForSignal(t *testing.T, ch <-chan struct{}, failureMsg string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal(failureMsg)
	}
}

func waitForExitCode(t *testing.T, exitCh <-chan int) int {
	t.Helper()

	select {
	case exitCode := <-exitCh:
		return exitCode
	case <-time.After(2 * time.Second):
		t.Fatal("application did not exit")
	}

	return 0
}
