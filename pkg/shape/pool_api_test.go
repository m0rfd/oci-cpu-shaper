package shape_test

import (
	"math"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/shape"
)

func TestPoolWorkersAndQuantumAccessors(t *testing.T) {
	t.Parallel()

	pool, err := shape.NewPool(3, 2*time.Millisecond)
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

	pool, err := shape.NewPool(1, 0)
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

func TestNewPoolRejectsNonPositiveWorkerCount(t *testing.T) {
	t.Parallel()

	_, err := shape.NewPool(0, shape.DefaultQuantum)
	if err == nil {
		t.Fatal("expected error when worker count is non-positive")
	}
}

func TestNewPoolClampsQuantumWithinBounds(t *testing.T) {
	t.Parallel()

	tooSmall, err := shape.NewPool(1, time.Microsecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := tooSmall.Quantum(); got != shape.DefaultQuantum {
		t.Fatalf("expected quantum to clamp to %s, got %s", shape.DefaultQuantum, got)
	}

	tooLarge, err := shape.NewPool(1, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	const maxQuantum = 5 * time.Millisecond
	if got := tooLarge.Quantum(); got != maxQuantum {
		t.Fatalf("expected quantum to clamp to %s, got %s", maxQuantum, got)
	}
}
