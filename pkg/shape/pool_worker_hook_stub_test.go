//go:build !rootful

//nolint:testpackage // test covers unexported worker hook stub.
package shape

import (
	"errors"
	"reflect"
	"testing"
)

var (
	errExistingInit = errors.New("existing init err")
	errInit         = errors.New("init err")
)

func TestConfigureWorkerStartHookAcceptsNilInputs(t *testing.T) {
	t.Parallel()

	configureWorkerStartHook(nil, nil)
}

func TestConfigureWorkerStartHookLeavesPoolUnchanged(t *testing.T) {
	t.Parallel()

	sentinelHook := func() error { return nil }
	sentinelHandler := func(error) {}
	pool := new(Pool)
	pool.workerStartHook = sentinelHook
	pool.workerStartErrorHandler = sentinelHandler
	pool.rootfulInitErr = errExistingInit

	configureWorkerStartHook(pool, nil)

	hookPtr := reflect.ValueOf(pool.workerStartHook).Pointer()

	sentinelHookPtr := reflect.ValueOf(sentinelHook).Pointer()
	if hookPtr != sentinelHookPtr {
		t.Fatalf("expected worker start hook to remain unchanged")
	}

	handlerPtr := reflect.ValueOf(pool.workerStartErrorHandler).Pointer()

	sentinelHandlerPtr := reflect.ValueOf(sentinelHandler).Pointer()
	if handlerPtr != sentinelHandlerPtr {
		t.Fatalf("expected worker start error handler to remain unchanged")
	}

	if !errors.Is(pool.rootfulInitErr, errExistingInit) {
		t.Fatalf("expected rootful init error to remain unchanged")
	}
}

func TestConfigureWorkerStartHookIsNoop(t *testing.T) {
	t.Parallel()

	initErr := errInit
	pool := new(Pool)

	configureWorkerStartHook(pool, initErr)

	if pool.workerStartHook != nil {
		t.Fatalf("expected worker start hook to remain unset")
	}

	if pool.workerStartErrorHandler != nil {
		t.Fatalf("expected worker start error handler to remain unset")
	}

	if pool.rootfulInitErr != nil {
		t.Fatalf("expected rootful init error to remain nil")
	}

	if initErr == nil {
		t.Fatalf("expected init err to remain unchanged")
	}
}
