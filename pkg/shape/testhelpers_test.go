//nolint:testpackage // helpers exercise internal worker seams
package shape

import (
	"sync/atomic"
	"testing"
	"time"
)

func assertBusyAndSleepDurations(
	t *testing.T,
	busyDurations []time.Duration,
	sleepDurations []time.Duration,
	quantum time.Duration,
) {
	t.Helper()

	if len(busyDurations) == 0 {
		t.Fatalf("expected busy durations to be recorded")
	}

	if len(busyDurations) != len(sleepDurations) {
		t.Fatalf("busy and sleep slices should match in length")
	}

	for index := range busyDurations {
		if busyDurations[index] <= 0 {
			t.Fatalf("expected positive busy duration")
		}

		if busyDurations[index] >= quantum {
			t.Fatalf("busy duration should be less than quantum: got %v", busyDurations[index])
		}

		if sleepDurations[index] <= 0 {
			t.Fatalf("expected positive sleep duration")
		}

		if busyDurations[index]+sleepDurations[index] != quantum {
			t.Fatalf(
				"quantum not preserved: busy %v sleep %v",
				busyDurations[index],
				sleepDurations[index],
			)
		}
	}
}

func waitForBusyCount(tb testing.TB, manual *manualTicker, counter *atomic.Int32, expected int32) {
	tb.Helper()
	manual.tick()

	deadline := time.Now().Add(25 * time.Millisecond)
	for time.Now().Before(deadline) {
		if counter.Load() >= expected {
			return
		}

		time.Sleep(500 * time.Microsecond)
	}

	tb.Fatalf("expected busy count >= %d, got %d", expected, counter.Load())
}

type manualTicker struct {
	ch chan time.Time
}

func newManualTicker() *manualTicker {
	return &manualTicker{ch: make(chan time.Time)}
}

func (m *manualTicker) C() <-chan time.Time {
	return m.ch
}

func (m *manualTicker) Stop() {
	close(m.ch)
}

func (m *manualTicker) tick() {
	m.ch <- time.Now()
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
