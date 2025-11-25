// Package adapt keeps constructor tests near the controller wiring to document
// the internal expectations.
//
//nolint:testpackage,godoclint // Tests exercise private helpers and provide file-level docs.
package adapt

import (
	"testing"
	"time"
)

func TestNewAdaptiveControllerInitializesRecorder(t *testing.T) {
	t.Parallel()

	recorder := newStubMetricsRecorder()
	metrics := newFakeMetrics(
		[]metricResult{{value: 0.2, timestamp: time.Unix(1_700_000_000, 0), err: nil}},
	)
	shaper := newFakeShaper()
	cfg := DefaultConfig()
	cfg.Mode = " dry-run "

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, recorder)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	requireEqual(t, "initialState", recorder.state, StateFallback.String())
	requireFloatApprox(t, "initialTarget", recorder.target, cfg.FallbackTarget)
	requirePositiveInt(t, "targetCalls", recorder.targetCalls)
	requireEqual(t, "initialInterval", recorder.interval, cfg.Interval)
	requirePositiveInt(t, "intervalCalls", recorder.intervalSet)
	requireEqual(t, "initialLastError", recorder.lastError, nil)
	requirePositiveInt(t, "errorCalls", recorder.errorCalls)
	requireEqual(t, "mode", recorder.mode, controller.Mode())
}

func TestNewAdaptiveControllerSetsFallbackTarget(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics(
		[]metricResult{{value: 0.3, timestamp: time.Unix(1_700_000_060, 0), err: nil}},
	)
	shaper := newFakeShaper()
	cfg := DefaultConfig()
	cfg.FallbackTarget = 0.42
	cfg.TargetMax = 0.7
	cfg.SuppressThreshold = 0.9
	cfg.SuppressResume = 0.75
	cfg.Mode = " enforce "

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	requireFloatApprox(t, "controllerTarget", controller.Target(), cfg.FallbackTarget)
	requireFloatApprox(t, "shaperTarget", shaper.Target(), cfg.FallbackTarget)
}
