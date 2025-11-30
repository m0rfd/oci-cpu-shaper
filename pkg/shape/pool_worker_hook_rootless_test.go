//go:build !rootful

//nolint:testpackage // tests need direct access to the unexported stub hook.
package shape

import (
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestConfigureWorkerStartHookRootlessNoop(t *testing.T) {
	t.Parallel()

	// Ensure the hook is a no-op when invoked with a nil pool and non-nil error.
	configureWorkerStartHook(nil, assertableError("ignored"))

	pool, err := NewPool(1, DefaultQuantum)
	if err != nil {
		t.Fatalf("unexpected error creating pool: %v", err)
	}

	// Wire minimal hooks to observe worker activity without altering behavior.
	manualTicker := newControllableTicker()
	pool.tickerFactory = func(time.Duration) ticker {
		return manualTicker
	}

	var (
		busyCalls  atomic.Int32
		sleepCalls atomic.Int32
	)

	pool.busyFunc = func(time.Duration) {
		busyCalls.Add(1)
	}
	pool.sleepFunc = func(time.Duration) {
		sleepCalls.Add(1)
	}

	ctx := t.Context()

	pool.Start(ctx)

	// Calling the hook again should remain a no-op, leaving pool behavior unchanged.
	configureWorkerStartHook(pool, assertableError("noop"))

	manualTicker.Tick()
	time.Sleep(5 * time.Millisecond)

	if busyCalls.Load() == 0 && sleepCalls.Load() == 0 {
		t.Fatalf("expected worker activity after start hook noop")
	}
}

func TestConfigureWorkerStartHookRootlessPoolConstructionNoop(t *testing.T) {
	t.Parallel()

	pool, err := NewPool(1, DefaultQuantum)
	if err != nil {
		t.Fatalf("unexpected error creating pool: %v", err)
	}

	originalHookPtr := functionPointer(pool.workerStartHook)
	originalHandlerPtr := functionPointer(pool.workerStartErrorHandler)
	originalRootfulErr := pool.rootfulInitErr

	configureWorkerStartHook(pool, assertableError("noop"))

	if functionPointer(pool.workerStartHook) != originalHookPtr {
		t.Fatalf("worker start hook should remain unchanged in rootless builds")
	}

	if functionPointer(pool.workerStartErrorHandler) != originalHandlerPtr {
		t.Fatalf("worker start error handler should remain unchanged in rootless builds")
	}

	if !errors.Is(pool.rootfulInitErr, originalRootfulErr) {
		t.Fatalf("rootful init error should remain unchanged in rootless builds")
	}
}

func functionPointer(fn any) uintptr {
	if fn == nil {
		return 0
	}

	return reflect.ValueOf(fn).Pointer()
}
