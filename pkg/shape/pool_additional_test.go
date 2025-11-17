//nolint:testpackage // coverage for internal hooks requires direct access.
package shape

import (
	"context"
	"errors"
	"math"
	"sync/atomic"
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

func TestPoolWorkerSkipsSleepWhenTargetIsFullyBusy(t *testing.T) {
	t.Parallel()

	pool, err := NewPool(1, 2*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.SetTarget(1)

	busyDurations, sleepCalls, yieldCalls := installFullyBusyWorkerProbes(pool)

	tick := newControllableTicker()
	pool.tickerFactory = func(time.Duration) ticker {
		return tick
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go pool.worker(ctx)

	tick.Tick()

	var busyDur time.Duration
	select {
	case busyDur = <-busyDurations:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("worker did not process tick")
	}

	cancel()
	time.Sleep(5 * time.Millisecond)

	if busyDur != pool.quantum {
		t.Fatalf("expected busy duration %s, got %s", pool.quantum, busyDur)
	}

	if got := sleepCalls.Load(); got != 0 {
		t.Fatalf("expected sleep to be skipped, got %d calls", got)
	}

	if got := yieldCalls.Load(); got < 2 {
		t.Fatalf("expected at least two yields when idle duration is zero, got %d", got)
	}
}

func installFullyBusyWorkerProbes(pool *Pool) (chan time.Duration, *atomic.Int32, *atomic.Int32) {
	busyDurations := make(chan time.Duration, 1)

	var (
		sleepCalls atomic.Int32
		yieldCalls atomic.Int32
	)

	pool.busyFunc = func(d time.Duration) {
		select {
		case busyDurations <- d:
		default:
		}
	}

	pool.sleepFunc = func(time.Duration) {
		sleepCalls.Add(1)
	}

	pool.yieldFunc = func() {
		yieldCalls.Add(1)
	}

	return busyDurations, &sleepCalls, &yieldCalls
}

type controllableTicker struct {
	ch chan time.Time
}

func newControllableTicker() *controllableTicker {
	return &controllableTicker{ch: make(chan time.Time, 1)}
}

func (t *controllableTicker) C() <-chan time.Time {
	return t.ch
}

func (t *controllableTicker) Stop() {}

func (t *controllableTicker) Tick() {
	t.ch <- time.Now()
}
