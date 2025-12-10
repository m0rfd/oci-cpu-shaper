//go:build !rootful

//nolint:testpackage // test covers unexported worker hook stub.
package shape

import (
	"errors"
	"reflect"
	"testing"
)

var errExistingInit = errors.New("existing init err")

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
