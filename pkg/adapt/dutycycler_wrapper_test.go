// Package adapt validates the duty-cycler wrappers to ensure dry-run behaviour
// matches the production expectations.
//
//nolint:testpackage,godoclint // Tests touch private helpers and document per-file coverage.
package adapt

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"
)

func TestAdaptiveControllerDryRunRecordsTargets(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.20, err: nil}}) //nolint:exhaustruct
	shaper := newFakeShaper()
	cfg := DefaultConfig()
	cfg.Mode = dryRunModeLabel
	cfg.Interval = time.Second

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("create controller: %v", err)
	}

	recorder, ok := controller.shaper.(*recordingDutyCycler)
	if !ok {
		t.Fatalf("expected dry-run to wrap duty cycler, got %T", controller.shaper)
	}

	if len(shaper.calls) != 0 {
		t.Fatalf("expected dry-run to avoid touching shaper, got %d calls", len(shaper.calls))
	}

	controller.step(context.Background())

	if len(shaper.calls) != 0 {
		t.Fatalf("expected dry-run to skip shaper updates, got %d calls", len(shaper.calls))
	}

	if recorder.Target() == 0 {
		t.Fatal("expected recorder to capture controller target in dry-run mode")
	}

	if diff := math.Abs(recorder.Target() - controller.Target()); diff > 1e-9 {
		t.Fatalf(
			"expected recorder target %.3f to match controller target %.3f",
			recorder.Target(),
			controller.Target(),
		)
	}

	recorder.ObserveHostLoad(0.85, 0)

	if len(shaper.calls) != 0 {
		t.Fatalf(
			"expected dry-run recorder to ignore host load observations, got %d calls",
			len(shaper.calls),
		)
	}
}

func TestAdaptiveControllerEnforceModeMutatesDutyCycler(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.20, err: nil}}) //nolint:exhaustruct
	shaper := newFakeShaper()
	cfg := DefaultConfig()
	cfg.Mode = enforceMode
	cfg.Interval = time.Second

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("create controller: %v", err)
	}

	if _, ok := controller.shaper.(*recordingDutyCycler); ok {
		t.Fatal("expected enforcing mode to use original duty cycler")
	}

	if len(shaper.calls) == 0 {
		t.Fatal("expected enforcing mode to configure fallback target")
	}

	controller.step(context.Background())

	if len(shaper.calls) < 2 {
		t.Fatalf(
			"expected enforcing mode to update shaper on step, got %d calls",
			len(shaper.calls),
		)
	}
}

func TestRecordingDutyCyclerObserveHostLoadNoop(t *testing.T) {
	t.Parallel()

	var nilRecorder *recordingDutyCycler
	nilRecorder.ObserveHostLoad(0.7, 0.5)

	recorder := &recordingDutyCycler{mu: sync.Mutex{}, target: 0}
	recorder.ObserveHostLoad(0.9, 0)
}

func TestNewModeAwareDutyCyclerHandlesNilShaper(t *testing.T) {
	t.Parallel()

	if got := newModeAwareDutyCycler(dryRunModeLabel, nil); got != nil {
		t.Fatalf("expected nil shaper to return nil wrapper, got %T", got)
	}
}

func TestRecordingDutyCyclerNilReceiver(t *testing.T) {
	t.Parallel()

	var recorder *recordingDutyCycler

	recorder.SetTarget(0.55)
	recorder.ObserveHostLoad(0.75, 0.1)

	if target := recorder.Target(); target != 0 {
		t.Fatalf("expected nil recorder target to remain zero, got %.2f", target)
	}
}
