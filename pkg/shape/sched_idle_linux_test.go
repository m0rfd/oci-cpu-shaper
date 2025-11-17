//go:build linux && rootful

package shape

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func TestTrySchedIdleSuccess(t *testing.T) {
	t.Parallel()

	schedSetAttrMu.Lock()
	original := schedSetAttr
	schedSetAttrMu.Unlock()

	t.Cleanup(func() {
		schedSetAttrMu.Lock()
		schedSetAttr = original
		schedSetAttrMu.Unlock()
	})

	var called bool
	schedSetAttrMu.Lock()
	schedSetAttr = func(pid int, attr *unix.SchedAttr, flags uint) error {
		called = true

		if pid != 0 {
			t.Fatalf("expected pid 0, got %d", pid)
		}

		if flags != 0 {
			t.Fatalf("expected zero flags, got %d", flags)
		}

		if attr == nil {
			t.Fatalf("expected non-nil sched attr")
		}

		if attr.Size == 0 {
			t.Fatalf("expected non-zero attr size")
		}

		if attr.Policy != unix.SCHED_IDLE {
			t.Fatalf("expected SCHED_IDLE policy, got %d", attr.Policy)
		}

		return nil
	}
	schedSetAttrMu.Unlock()

	if err := trySchedIdle(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !called {
		t.Fatalf("expected schedSetScheduler to be called")
	}
}

func TestTrySchedIdleEPERM(t *testing.T) {
	t.Parallel()

	schedSetAttrMu.Lock()
	original := schedSetAttr
	schedSetAttrMu.Unlock()

	t.Cleanup(func() {
		schedSetAttrMu.Lock()
		schedSetAttr = original
		schedSetAttrMu.Unlock()
	})

	schedSetAttrMu.Lock()
	schedSetAttr = func(int, *unix.SchedAttr, uint) error {
		return unix.EPERM
	}
	schedSetAttrMu.Unlock()

	err := trySchedIdle()

	if !errors.Is(err, unix.EPERM) {
		t.Fatalf("expected EPERM, got %v", err)
	}
}
