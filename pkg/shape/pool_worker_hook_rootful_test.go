//go:build linux && rootful

package shape

import (
	"errors"
	"reflect"
	"testing"
)

func TestNewPoolInstallsWorkerStartHookWhenSchedIdleSucceeds(t *testing.T) {
	restoreSetter := withFakeSchedIdleSetter(nil)
	t.Cleanup(restoreSetter)

	originalHandler := defaultWorkerStartErrorHandler
	handlerCalls := 0
	defaultWorkerStartErrorHandler = func(err error) {
		handlerCalls++

		t.Fatalf("unexpected worker start error: %v", err)
	}
	t.Cleanup(func() {
		defaultWorkerStartErrorHandler = originalHandler
	})

	pool, err := NewPool(1, DefaultQuantum)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pool.workerStartHook == nil {
		t.Fatalf("expected worker start hook to be configured")
	}

	if got, want := reflect.ValueOf(pool.workerStartHook).Pointer(), reflect.ValueOf(trySchedIdle).Pointer(); got != want {
		t.Fatalf("expected worker start hook to point to trySchedIdle: got %v, want %v", got, want)
	}

	if handlerCalls != 0 {
		t.Fatalf("expected worker start error handler to remain unused, got %d call(s)", handlerCalls)
	}
}

func TestNewPoolLeavesStartHookUnsetWhenSchedIdleFails(t *testing.T) {
	fakeError := errors.New("sched_idle unavailable")
	fake := &fakeSchedIdleSetter{err: fakeError}
	restoreSetter := withFakeSchedIdleSetter(fake)
	t.Cleanup(restoreSetter)

	originalHandler := defaultWorkerStartErrorHandler
	handlerCalls := 0
	var receivedErr error
	defaultWorkerStartErrorHandler = func(err error) {
		handlerCalls++
		receivedErr = err
	}
	t.Cleanup(func() {
		defaultWorkerStartErrorHandler = originalHandler
	})

	pool, err := NewPool(1, DefaultQuantum)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pool.workerStartHook != nil {
		t.Fatalf("expected worker start hook to remain unset when sched_idle fails")
	}

	if handlerCalls != 1 {
		t.Fatalf("expected worker start error handler to be invoked once, got %d call(s)", handlerCalls)
	}

	if !errors.Is(receivedErr, fakeError) {
		t.Fatalf("expected error handler to receive sched_idle failure, got %v", receivedErr)
	}
}
