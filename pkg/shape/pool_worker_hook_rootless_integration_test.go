//go:build !rootful

//nolint:testpackage // tests need access to workerStartHook.
package shape

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestConfigureWorkerStartHookRootlessIntegration(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	pool, err := NewPool(1, DefaultQuantum)
	if err != nil {
		t.Fatalf("unexpected error creating pool: %v", err)
	}

	var handlerCalls atomic.Int32

	handlerErrors := make(chan error, 1)

	pool.SetWorkerStartErrorHandler(func(err error) {
		handlerCalls.Add(1)

		handlerErrors <- err
	})

	configureWorkerStartHook(pool, nil)
	configureWorkerStartHook(pool, assertableError("ignored"))

	if handlerCalls.Load() != 0 {
		t.Fatalf("worker start error handler invoked before workers were started")
	}

	expectedHookErr := assertableError("start hook failed")
	pool.workerStartHook = func() error {
		return expectedHookErr
	}

	pool.Start(ctx)

	select {
	case err := <-handlerErrors:
		if !errors.Is(err, expectedHookErr) {
			t.Fatalf("unexpected worker start error: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("worker start error handler was not invoked")
	}

	cancel()
	time.Sleep(5 * time.Millisecond)

	if handlerCalls.Load() != 1 {
		t.Fatalf("worker start error handler was invoked more than once: %d", handlerCalls.Load())
	}
}
