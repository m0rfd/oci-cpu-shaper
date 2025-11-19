package shape_test

import (
	"math"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/shape"
)

func TestPoolObserveHostLoadTransitions(t *testing.T) {
	t.Parallel()

	pool, err := shape.NewPool(1, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.SetPauseThresholds(0.75, 0.5)

	pool.ObserveHostLoad(0.25)

	if pool.Paused() {
		t.Fatalf("expected workers to remain active below threshold")
	}

	pool.ObserveHostLoad(0.80)

	if !pool.Paused() {
		t.Fatalf("expected workers to pause once utilisation crosses threshold")
	}

	pool.ObserveHostLoad(0.60)

	if !pool.Paused() {
		t.Fatalf("expected workers to remain paused until utilisation cools")
	}

	pool.ObserveHostLoad(0.30)

	if pool.Paused() {
		t.Fatalf("expected workers to resume once utilisation falls below resume threshold")
	}

	pool.SetPauseThresholds(0, 0)
	pool.ObserveHostLoad(0.99)

	if pool.Paused() {
		t.Fatalf("expected disabling thresholds to resume workers")
	}
}

func TestPoolSetPauseThresholdsResetOnNaN(t *testing.T) {
	t.Parallel()

	pool, err := shape.NewPool(1, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.SetPauseThresholds(0.5, 0.2)
	pool.ObserveHostLoad(0.7)

	if !pool.Paused() {
		t.Fatalf("expected workers to pause before resetting thresholds")
	}

	pool.SetPauseThresholds(math.NaN(), math.NaN())

	if pool.Paused() {
		t.Fatalf("expected NaN thresholds to reset paused state")
	}

	pool.ObserveHostLoad(0.8)

	if pool.Paused() {
		t.Fatalf("expected zero thresholds to disable pausing")
	}
}

func TestPoolSetPauseThresholdsClampAndCap(t *testing.T) {
	t.Parallel()

	pool, err := shape.NewPool(1, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.SetPauseThresholds(1.5, -0.3)

	pool.ObserveHostLoad(0.99)

	if pool.Paused() {
		t.Fatalf("expected utilisation below clamped pause threshold to remain active")
	}

	pool.ObserveHostLoad(1)

	if !pool.Paused() {
		t.Fatalf("expected utilisation at clamped pause threshold to pause workers")
	}

	pool.ObserveHostLoad(0.4)

	if !pool.Paused() {
		t.Fatalf("expected resume threshold clamped to zero to keep workers paused")
	}

	pool.ObserveHostLoad(0)

	if pool.Paused() {
		t.Fatalf("expected zero utilisation to resume workers when resume threshold is zero")
	}

	pool.SetPauseThresholds(0.4, 0.9)

	pool.ObserveHostLoad(0.6)

	if !pool.Paused() {
		t.Fatalf("expected workers to pause when utilisation exceeds pause threshold")
	}

	pool.ObserveHostLoad(0.35)

	if pool.Paused() {
		t.Fatalf("expected resume threshold to cap at pause threshold")
	}
}

func TestPoolObserveHostLoadNormalisesInput(t *testing.T) {
	t.Parallel()

	pool, err := shape.NewPool(1, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.SetPauseThresholds(0.6, 0.2)

	pool.ObserveHostLoad(math.NaN())

	if pool.Paused() {
		t.Fatalf("expected NaN observation to leave state unchanged")
	}

	pool.ObserveHostLoad(math.Inf(1))

	if pool.Paused() {
		t.Fatalf("expected Inf observation to leave state unchanged")
	}

	pool.ObserveHostLoad(0.7)

	if !pool.Paused() {
		t.Fatalf("expected valid utilisation above threshold to pause workers")
	}

	pool.ObserveHostLoad(math.Inf(1))

	if !pool.Paused() {
		t.Fatalf("expected Inf observation to leave paused state unchanged")
	}

	pool.ObserveHostLoad(-0.5)

	if pool.Paused() {
		t.Fatalf("expected negative utilisation to clamp to resume threshold and resume workers")
	}

	pool.ObserveHostLoad(2)

	if !pool.Paused() {
		t.Fatalf("expected utilisation above 1 to clamp and pause workers")
	}
}
