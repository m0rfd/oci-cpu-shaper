//nolint:testpackage // tests require access to unexported hooks
package shape

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var (
	errTestSchedIdleDenied = errors.New("sched idle denied")
	errTestSchedIdle       = errors.New("sched_idle failed")
)

func TestPoolAppliesDutyCycle(t *testing.T) {
	t.Parallel()

	pool, err := NewPool(1, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var (
		metricsMu      sync.Mutex
		busyDurations  []time.Duration
		sleepDurations []time.Duration
	)

	pool.busyFunc = func(d time.Duration) {
		metricsMu.Lock()

		busyDurations = append(busyDurations, d)

		metricsMu.Unlock()
	}

	pool.sleepFunc = func(d time.Duration) {
		metricsMu.Lock()

		sleepDurations = append(sleepDurations, d)

		metricsMu.Unlock()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.Start(ctx)
	pool.SetTarget(0.4)

	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(2 * time.Millisecond)

	metricsMu.Lock()
	defer metricsMu.Unlock()

	assertBusyAndSleepDurations(t, busyDurations, sleepDurations, 5*time.Millisecond)
}

func TestPoolYieldsUnderZeroTarget(t *testing.T) {
	t.Parallel()

	pool, err := NewPool(1, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var yieldCount atomic.Int64

	pool.yieldFunc = func() {
		yieldCount.Add(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.Start(ctx)
	pool.SetTarget(0)

	time.Sleep(5 * time.Millisecond)
	cancel()
	time.Sleep(2 * time.Millisecond)

	if yieldCount.Load() == 0 {
		t.Fatalf("expected yields when target is zero")
	}
}

func TestPoolPausesWorkerActivityWhenThresholdExceeded(t *testing.T) {
	t.Parallel()

	pool, err := NewPool(1, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	manual := newManualTicker()
	pool.tickerFactory = func(time.Duration) ticker {
		return manual
	}
	pool.sleepFunc = func(time.Duration) {}
	pool.yieldFunc = func() {}

	pool.SetTarget(1)
	pool.SetPauseThresholds(0.7, 0.4)

	var busyCount atomic.Int32

	pool.busyFunc = func(time.Duration) {
		busyCount.Add(1)
	}

	ctx := t.Context()

	pool.Start(ctx)

	waitForBusyCount(t, manual, &busyCount, 1)

	pool.ObserveHostLoad(0.9)
	manual.tick()
	time.Sleep(2 * time.Millisecond)

	pausedCount := busyCount.Load()

	manual.tick()
	time.Sleep(2 * time.Millisecond)

	if busyCount.Load() != pausedCount {
		t.Fatalf(
			"expected busy work to stop while paused; got %d want %d",
			busyCount.Load(),
			pausedCount,
		)
	}

	pool.ObserveHostLoad(0.3)
	waitForBusyCount(t, manual, &busyCount, pausedCount+1)
}

func TestPoolWorkerStartHookSuccess(t *testing.T) {
	t.Parallel()

	const workers = 3

	pool, err := NewPool(workers, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var (
		hookCount        atomic.Int32
		handlerCount     atomic.Int32
		workerStartGroup sync.WaitGroup
	)

	workerStartGroup.Add(workers)

	pool.workerStartHook = func() error {
		hookCount.Add(1)
		workerStartGroup.Done()

		return nil
	}
	pool.workerStartErrorHandler = func(error) {
		handlerCount.Add(1)
	}
	pool.sleepFunc = func(time.Duration) {}
	pool.yieldFunc = func() {}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.Start(ctx)

	done := make(chan struct{})

	go func() {
		workerStartGroup.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("timeout waiting for worker start hook")
	}

	cancel()
	time.Sleep(2 * time.Millisecond)

	if got := hookCount.Load(); got != workers {
		t.Fatalf("expected hook count %d, got %d", workers, got)
	}

	if got := handlerCount.Load(); got != 0 {
		t.Fatalf("expected no error handler invocations, got %d", got)
	}
}

//nolint:funlen // integration-style test ensures handler runs per worker
func TestPoolWorkerStartHookErrorPropagates(t *testing.T) {
	t.Parallel()

	const workers = 2

	pool, err := NewPool(workers, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var (
		hookCount    atomic.Int32
		handlerCount atomic.Int32
	)

	hookErr := errTestSchedIdleDenied

	var handlerWG sync.WaitGroup
	handlerWG.Add(workers)

	pool.workerStartHook = func() error {
		hookCount.Add(1)

		return hookErr
	}
	pool.workerStartErrorHandler = func(err error) {
		if !errors.Is(err, hookErr) {
			t.Errorf("unexpected error propagated: %v", err)
		}

		handlerCount.Add(1)
		handlerWG.Done()
	}
	pool.sleepFunc = func(time.Duration) {}
	pool.yieldFunc = func() {}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.Start(ctx)

	done := make(chan struct{})

	go func() {
		handlerWG.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("timeout waiting for worker start error handler")
	}

	cancel()
	time.Sleep(2 * time.Millisecond)

	if got := hookCount.Load(); got != workers {
		t.Fatalf("expected hook count %d, got %d", workers, got)
	}

	if got := handlerCount.Load(); got != workers {
		t.Fatalf("expected handler count %d, got %d", workers, got)
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
