//go:build rootful

package shape

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

type fakeSchedIdleSetter struct {
	called bool
	err    error
}

func (f *fakeSchedIdleSetter) setScheduler(pid int, policy int, param *schedParam) error {
	f.called = true

	if pid != 0 {
		return errors.New("unexpected pid")
	}

	if policy != unix.SCHED_IDLE {
		return errors.New("unexpected policy")
	}

	if param == nil {
		return errors.New("missing sched param")
	}

	return f.err
}

func withFakeSchedIdleSetter(setter schedIdleSetter) func() {
	schedIdleSetterMu.Lock()
	original := currentSchedIdleSetter
	currentSchedIdleSetter = setter
	schedIdleSetterMu.Unlock()

	return func() {
		schedIdleSetterMu.Lock()
		currentSchedIdleSetter = original
		schedIdleSetterMu.Unlock()
	}
}

func TestTrySchedIdleSuccess(t *testing.T) {
	t.Parallel()

	fake := &fakeSchedIdleSetter{}
	restore := withFakeSchedIdleSetter(fake)
	t.Cleanup(restore)

	if err := trySchedIdle(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fake.called {
		t.Fatalf("expected fake scheduler to be invoked")
	}
}

func TestTrySchedIdlePropagatesEPERM(t *testing.T) {
	t.Parallel()

	fake := &fakeSchedIdleSetter{err: unix.EPERM}
	restore := withFakeSchedIdleSetter(fake)
	t.Cleanup(restore)

	if err := trySchedIdle(); !errors.Is(err, unix.EPERM) {
		t.Fatalf("expected EPERM, got %v", err)
	}
}

func TestTrySchedIdlePropagatesUnexpectedError(t *testing.T) {
	t.Parallel()

	fake := &fakeSchedIdleSetter{err: errors.New("boom")}
	restore := withFakeSchedIdleSetter(fake)
	t.Cleanup(restore)

	if err := trySchedIdle(); !errors.Is(err, fake.err) {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

func TestConfigureWorkerStartHookSetsHookOnSuccess(t *testing.T) {
	t.Parallel()

	pool := &Pool{}
	configureWorkerStartHook(pool, nil)

	if pool.workerStartHook == nil {
		t.Fatalf("expected worker start hook to be configured")
	}
}

func TestConfigureWorkerStartHookSkipsOnError(t *testing.T) {
	t.Parallel()

	pool := &Pool{}
	configureWorkerStartHook(pool, errors.New("denied"))

	if pool.workerStartHook != nil {
		t.Fatalf("expected hook to remain unset when sched_idle is unavailable")
	}
}

func TestNewPoolWarnsAndReplaysSchedIdleError(t *testing.T) {
	t.Parallel()

	fake := &fakeSchedIdleSetter{err: unix.EPERM}
	restore := withFakeSchedIdleSetter(fake)
	t.Cleanup(restore)

	originalDefaultHandler := defaultWorkerStartErrorHandler

	defaultCalls := 0
	var defaultErr error
	defaultWorkerStartErrorHandler = func(err error) {
		defaultCalls++
		defaultErr = err
	}

	t.Cleanup(func() {
		defaultWorkerStartErrorHandler = originalDefaultHandler
	})

	pool, err := NewPool(1, DefaultQuantum)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if defaultCalls != 1 {
		t.Fatalf("expected default handler to be called once, got %d", defaultCalls)
	}

	if !errors.Is(defaultErr, unix.EPERM) {
		t.Fatalf("expected default handler to receive EPERM, got %v", defaultErr)
	}

	if !errors.Is(pool.rootfulInitErr, unix.EPERM) {
		t.Fatalf("expected pool to retain sched_idle error, got %v", pool.rootfulInitErr)
	}

	customCalls := 0
	pool.SetWorkerStartErrorHandler(func(err error) {
		if !errors.Is(err, unix.EPERM) {
			t.Fatalf("expected custom handler to receive EPERM, got %v", err)
		}

		customCalls++
	})

	if customCalls != 1 {
		t.Fatalf("expected custom handler to be called once, got %d", customCalls)
	}

	if defaultCalls != 1 {
		t.Fatalf("expected default handler to avoid extra calls, got %d", defaultCalls)
	}

	if pool.rootfulInitErr != nil {
		t.Fatalf("expected pool to clear stored sched_idle error after replay, got %v", pool.rootfulInitErr)
	}
}
