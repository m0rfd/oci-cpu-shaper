//nolint:testpackage // exercising internal hooks and stored errors
package shape

import (
	"errors"
	"math"
	"testing"
)

var errSchedIdleDenied = errors.New("sched_idle denied")

//nolint:paralleltest // modifies global sched_idle hook state
func TestNewPoolIntegrationHandlesSchedIdleFailure(t *testing.T) {
	expectedErr := errSchedIdleDenied

	restoreHook := withTrySchedIdleHook(func() error {
		return expectedErr
	})
	t.Cleanup(restoreHook)

	pool, err := NewPool(2, DefaultQuantum)
	if err != nil {
		t.Fatalf("unexpected error creating pool: %v", err)
	}

	handlerCalls := 0

	var receivedErr error

	pool.SetWorkerStartErrorHandler(func(err error) {
		handlerCalls++
		receivedErr = err
	})

	if handlerCalls != 1 {
		t.Fatalf("expected worker start error handler to be invoked once, got %d", handlerCalls)
	}

	if !errors.Is(receivedErr, expectedErr) {
		t.Fatalf(
			"expected worker start error handler to receive %v, got %v",
			expectedErr,
			receivedErr,
		)
	}

	if pool.rootfulInitErr != nil {
		t.Fatalf(
			"expected rootful init error to be consumed after handler replay, got %v",
			pool.rootfulInitErr,
		)
	}

	if pool.Target() != 0 {
		t.Fatalf("expected pool target to default to 0, got %f", pool.Target())
	}

	if pause := math.Float64frombits(pool.pauseThresholdBits.Load()); pause != 0 {
		t.Fatalf("expected pause threshold to default to 0, got %f", pause)
	}

	if resume := math.Float64frombits(pool.resumeThresholdBits.Load()); resume != 0 {
		t.Fatalf("expected resume threshold to default to 0, got %f", resume)
	}

	if runnableGuard := math.Float64frombits(pool.runnableGuardBits.Load()); runnableGuard != 0 {
		t.Fatalf("expected runnable guard to default to 0, got %f", runnableGuard)
	}
}
