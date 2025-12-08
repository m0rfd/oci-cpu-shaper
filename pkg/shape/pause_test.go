//nolint:testpackage // access internal pool state for pause threshold assertions
package shape

import (
	"math"
	"testing"
	"time"
)

type pauseThresholdTestCase struct {
	name           string
	pause          float64
	resume         float64
	initialPaused  bool
	expectedPause  float64
	expectedResume float64
	expectedPaused bool
}

func TestPoolSetPauseThresholdsNormalisesAndStores(t *testing.T) {
	t.Parallel()

	testCases := []pauseThresholdTestCase{
		{
			name:           "NaN inputs reset thresholds",
			pause:          math.NaN(),
			resume:         math.NaN(),
			initialPaused:  true,
			expectedPause:  0,
			expectedResume: 0,
			expectedPaused: false,
		},
		{
			name:           "Infinite inputs clamp to one",
			pause:          math.Inf(1),
			resume:         math.Inf(1),
			initialPaused:  false,
			expectedPause:  1,
			expectedResume: 1,
			expectedPaused: false,
		},
		{
			name:           "Negative values clamp to zero",
			pause:          -0.4,
			resume:         -0.2,
			initialPaused:  true,
			expectedPause:  0,
			expectedResume: 0,
			expectedPaused: false,
		},
		{
			name:           "Resume exceeds pause caps to pause",
			pause:          0.3,
			resume:         0.8,
			initialPaused:  false,
			expectedPause:  0.3,
			expectedResume: 0.3,
			expectedPaused: false,
		},
		{
			name:           "Zero pause disables feature",
			pause:          0,
			resume:         0.6,
			initialPaused:  true,
			expectedPause:  0,
			expectedResume: 0,
			expectedPaused: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runPauseThresholdTestCase(t, testCase)
		})
	}
}

func runPauseThresholdTestCase(t *testing.T, testCase pauseThresholdTestCase) {
	t.Helper()

	pool, err := NewPool(1, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if testCase.initialPaused {
		pool.paused.Store(1)
	}

	pool.SetPauseThresholds(testCase.pause, testCase.resume)

	assertPauseThresholds(t, pool, testCase.expectedPause, testCase.expectedResume)
	assertPausedState(t, pool, testCase.expectedPaused)
}

func TestPoolObserveHostLoadTransitions(t *testing.T) {
	t.Parallel()

	pool, err := NewPool(1, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.SetPauseThresholds(0.75, 0.5)

	pool.ObserveHostLoad(0.25, 0)

	if pool.Paused() {
		t.Fatalf("expected workers to remain active below threshold")
	}

	pool.ObserveHostLoad(0.80, 0)

	if !pool.Paused() {
		t.Fatalf("expected workers to pause once utilisation crosses threshold")
	}

	pool.ObserveHostLoad(0.60, 0)

	if !pool.Paused() {
		t.Fatalf("expected workers to remain paused until utilisation cools")
	}

	pool.ObserveHostLoad(0.30, 0)

	if pool.Paused() {
		t.Fatalf("expected workers to resume once utilisation falls below resume threshold")
	}

	pool.SetPauseThresholds(0, 0)
	pool.ObserveHostLoad(0.99, 0)

	if pool.Paused() {
		t.Fatalf("expected disabling thresholds to resume workers")
	}
}

func TestPoolSetPauseThresholdsResetOnNaN(t *testing.T) {
	t.Parallel()

	pool, err := NewPool(1, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.SetPauseThresholds(0.5, 0.2)
	pool.ObserveHostLoad(0.7, 0)

	if !pool.Paused() {
		t.Fatalf("expected workers to pause before resetting thresholds")
	}

	pool.SetPauseThresholds(math.NaN(), math.NaN())

	if pool.Paused() {
		t.Fatalf("expected NaN thresholds to reset paused state")
	}

	pool.ObserveHostLoad(0.8, 0)

	if pool.Paused() {
		t.Fatalf("expected zero thresholds to disable pausing")
	}
}

func TestPoolSetPauseThresholdsClampAndCap(t *testing.T) {
	t.Parallel()

	pool, err := NewPool(1, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.SetPauseThresholds(1.5, -0.3)

	pool.ObserveHostLoad(0.99, 0)

	if pool.Paused() {
		t.Fatalf("expected utilisation below clamped pause threshold to remain active")
	}

	pool.ObserveHostLoad(1, 0)

	if !pool.Paused() {
		t.Fatalf("expected utilisation at clamped pause threshold to pause workers")
	}

	pool.ObserveHostLoad(0.4, 0)

	if !pool.Paused() {
		t.Fatalf("expected resume threshold clamped to zero to keep workers paused")
	}

	pool.ObserveHostLoad(0, 0)

	if pool.Paused() {
		t.Fatalf("expected zero utilisation to resume workers when resume threshold is zero")
	}

	pool.SetPauseThresholds(0.4, 0.9)

	pool.ObserveHostLoad(0.6, 0)

	if !pool.Paused() {
		t.Fatalf("expected workers to pause when utilisation exceeds pause threshold")
	}

	pool.ObserveHostLoad(0.35, 0)

	if pool.Paused() {
		t.Fatalf("expected resume threshold to cap at pause threshold")
	}
}

func TestPoolObserveHostLoadNormalisesInput(t *testing.T) {
	t.Parallel()

	pool, err := NewPool(1, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.SetPauseThresholds(0.6, 0.2)

	pool.ObserveHostLoad(math.NaN(), 0)

	if pool.Paused() {
		t.Fatalf("expected NaN observation to leave state unchanged")
	}

	pool.ObserveHostLoad(math.Inf(1), 0)

	if pool.Paused() {
		t.Fatalf("expected Inf observation to leave state unchanged")
	}

	pool.ObserveHostLoad(0.7, 0)

	if !pool.Paused() {
		t.Fatalf("expected valid utilisation above threshold to pause workers")
	}

	pool.ObserveHostLoad(math.Inf(1), 0)

	if !pool.Paused() {
		t.Fatalf("expected Inf observation to leave paused state unchanged")
	}

	pool.ObserveHostLoad(-0.5, 0)

	if pool.Paused() {
		t.Fatalf("expected negative utilisation to clamp to resume threshold and resume workers")
	}

	pool.ObserveHostLoad(2, 0)

	if !pool.Paused() {
		t.Fatalf("expected utilisation above 1 to clamp and pause workers")
	}
}

func TestPoolRunnableGuardPausesAndResumes(t *testing.T) {
	t.Parallel()

	pool, err := NewPool(1, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.SetRunnableGuard(1.2)
	pool.SetPauseThresholds(0, 0)

	pool.ObserveHostLoad(0.3, 1.5)

	if !pool.Paused() {
		t.Fatalf("expected runnable guard to pause workers when exceeded")
	}

	pool.ObserveHostLoad(0.3, 0.2)

	if pool.Paused() {
		t.Fatalf("expected workers to resume once runnable guard cools")
	}
}

func TestPoolObserveHostLoadNormalisesRunnable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		runnable     float64
		expectPaused bool
	}{
		{name: "nan", runnable: math.NaN(), expectPaused: false},
		{name: "pos-inf", runnable: math.Inf(1), expectPaused: false},
		{name: "neg-inf", runnable: math.Inf(-1), expectPaused: false},
		{name: "negative", runnable: -0.5, expectPaused: false},
		{name: "below-guard", runnable: 0.4, expectPaused: false},
		{name: "at-guard", runnable: 1, expectPaused: true},
		{name: "above-guard", runnable: 1.5, expectPaused: true},
	}

	for _, tt := range tests {
		testCase := tt
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pool, err := NewPool(1, time.Millisecond)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			pool.SetPauseThresholds(0, 0)
			pool.SetRunnableGuard(1)

			pool.ObserveHostLoad(0.2, testCase.runnable)

			if paused := pool.Paused(); paused != testCase.expectPaused {
				t.Fatalf(
					"expected paused=%t for runnable %v, got %t",
					testCase.expectPaused,
					testCase.runnable,
					paused,
				)
			}
		})
	}
}

func TestPoolRunnableGuardClampsInvalidInputs(t *testing.T) {
	t.Parallel()

	pool, err := NewPool(1, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.SetRunnableGuard(math.NaN())
	pool.ObserveHostLoad(0.4, math.Inf(1))

	if pool.Paused() {
		t.Fatalf("expected NaN runnable guard to disable pausing")
	}

	pool.SetRunnableGuard(-1)
	pool.ObserveHostLoad(0.4, 5)

	if pool.Paused() {
		t.Fatalf("expected negative runnable guard to disable pausing")
	}
}
