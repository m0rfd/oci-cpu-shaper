//go:build !rootful

//nolint:testpackage // tests need access to internal pool fields.
package shape

import (
	"errors"
	"reflect"
	"testing"
)

func TestConfigureWorkerStartHookRootlessPreservesPoolState(t *testing.T) {
	t.Parallel()

	pool, err := NewPool(1, DefaultQuantum)
	if err != nil {
		t.Fatalf("unexpected error creating pool: %v", err)
	}

	sentinelHookErr := assertableError("sentinel")

	var handlerCalled bool

	sentinelHook := func() error {
		return sentinelHookErr
	}
	sentinelHandler := func(err error) {
		handlerCalled = true

		if !errors.Is(err, sentinelHookErr) {
			t.Fatalf("unexpected error propagated: %v", err)
		}
	}

	pool.workerStartHook = sentinelHook
	pool.workerStartErrorHandler = sentinelHandler
	pool.rootfulInitErr = sentinelHookErr

	configureWorkerStartHook(pool, sentinelHookErr)

	if reflect.ValueOf(pool.workerStartHook).Pointer() != reflect.ValueOf(sentinelHook).Pointer() {
		t.Fatalf("worker start hook was modified by rootless configure hook")
	}

	if reflect.ValueOf(pool.workerStartErrorHandler).
		Pointer() !=
		reflect.ValueOf(sentinelHandler).
			Pointer() {
		t.Fatalf("worker start error handler was modified by rootless configure hook")
	}

	if !errors.Is(pool.rootfulInitErr, sentinelHookErr) {
		t.Fatalf("rootful init error was modified by rootless configure hook")
	}

	if handlerCalled {
		t.Fatalf("worker start error handler should not be invoked by rootless configure hook")
	}
}
