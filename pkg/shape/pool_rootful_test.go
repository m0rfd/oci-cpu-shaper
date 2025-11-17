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

func (f *fakeSchedIdleSetter) setScheduler(pid int, policy int, param *unix.SchedParam) error {
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

func TestTrySchedIdleIgnoresEPERM(t *testing.T) {
	t.Parallel()

	fake := &fakeSchedIdleSetter{err: unix.EPERM}
	restore := withFakeSchedIdleSetter(fake)
	t.Cleanup(restore)

	if err := trySchedIdle(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
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
