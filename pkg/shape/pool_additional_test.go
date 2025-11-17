//nolint:testpackage // coverage for internal hooks requires direct access.
package shape

import (
	"errors"
	"math"
	"testing"
	"time"
)

var errTestSchedIdle = errors.New("sched_idle failed")

func TestPoolWorkersAndQuantumAccessors(t *testing.T) {
	t.Parallel()

	pool, err := NewPool(3, 2*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := pool.Workers(); got != 3 {
		t.Fatalf("unexpected worker count: got %d want 3", got)
	}

	if got := pool.Quantum(); got != 2*time.Millisecond {
		t.Fatalf("unexpected quantum: got %s want %s", got, 2*time.Millisecond)
	}
}

func TestPoolSetTargetBoundsInput(t *testing.T) {
	t.Parallel()

	pool, err := NewPool(1, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.SetTarget(1.5)

	if got := pool.Target(); got != 1 {
		t.Fatalf("expected target to clamp to 1, got %.2f", got)
	}

	pool.SetTarget(-0.2)

	if got := pool.Target(); got != 0 {
		t.Fatalf("expected negative target to clamp to 0, got %.2f", got)
	}

	pool.SetTarget(math.NaN())

	if got := pool.Target(); got != 0 {
		t.Fatalf("expected NaN target to reset to 0, got %.2f", got)
	}
}

func TestSetWorkerStartErrorHandlerReplaysStoredError(t *testing.T) {
	t.Parallel()

	pool, err := NewPool(1, DefaultQuantum)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.rootfulInitErr = errTestSchedIdle

	var called int

	pool.SetWorkerStartErrorHandler(func(err error) {
		if err == nil {
			t.Fatalf("expected error, got nil")
		}

		called++
	})

	if called != 1 {
		t.Fatalf("expected handler to be called once, got %d", called)
	}

	pool.SetWorkerStartErrorHandler(func(error) {})

	if called != 1 {
		t.Fatalf("handler should not be called again, got %d", called)
	}
}

func TestNewPoolRejectsNonPositiveWorkerCount(t *testing.T) {
	t.Parallel()

	_, err := NewPool(0, DefaultQuantum)
	if err == nil {
		t.Fatal("expected error when worker count is non-positive")
	}
}

func TestNewPoolClampsQuantumWithinBounds(t *testing.T) {
	t.Parallel()

	tooSmall, err := NewPool(1, time.Microsecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := tooSmall.Quantum(); got != minQuantum {
		t.Fatalf("expected quantum to clamp to %s, got %s", minQuantum, got)
	}

	tooLarge, err := NewPool(1, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := tooLarge.Quantum(); got != maxQuantum {
		t.Fatalf("expected quantum to clamp to %s, got %s", maxQuantum, got)
	}
}
