// Package adapt keeps constructor tests near the controller wiring to document
// the internal expectations.
//
//nolint:testpackage,godoclint // Tests exercise private helpers and provide file-level docs.
package adapt

import (
	"errors"
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

func TestNewAdaptiveControllerNormalizesConfigAndRecordsInitialState(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	defaultCfg := cfg
	cfg.Mode = " DRY-RUN "
	cfg.Interval = 0
	cfg.FallbackTarget = 0

	recorder := newStubMetricsRecorder()
	metrics := newFakeMetrics(
		[]metricResult{{value: 0.2, timestamp: time.Unix(1_700_000_120, 0), err: nil}},
	)
	shaper := newFakeShaper()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, recorder)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	requireEqual(t, "mode", controller.Mode(), dryRunModeLabel)
	requireEqual(t, "recorderMode", recorder.mode, dryRunModeLabel)
	requireEqual(t, "state", controller.State(), StateFallback)
	requireEqual(t, "recorderState", recorder.state, StateFallback.String())
	requireFloatApprox(t, "target", controller.Target(), defaultCfg.FallbackTarget)
	requireFloatApprox(t, "recorderTarget", recorder.target, defaultCfg.FallbackTarget)
	requireEqual(t, "interval", controller.interval, defaultCfg.Interval)
	requireEqual(t, "recorderInterval", recorder.interval, defaultCfg.Interval)
}

func TestNewAdaptiveControllerRejectsNilMetricsClient(t *testing.T) {
	t.Parallel()

	recorder := newStubMetricsRecorder()

	controller, err := NewAdaptiveController(DefaultConfig(), nil, nil, newFakeShaper(), recorder)
	if err == nil {
		t.Fatalf("expected error, got controller: %+v", controller)
	}

	if !errors.Is(err, errMetricsClientRequired) {
		t.Fatalf("expected errMetricsClientRequired, got %v", err)
	}

	if recorderCalls(recorder) != 0 {
		t.Fatalf("recorder should not be called when metrics client is nil")
	}
}

func TestNewAdaptiveControllerRejectsNilDutyCycler(t *testing.T) {
	t.Parallel()

	recorder := newStubMetricsRecorder()
	metrics := newFakeMetrics(
		[]metricResult{{value: 0.2, timestamp: time.Unix(1_700_000_240, 0), err: nil}},
	)

	controller, err := NewAdaptiveController(DefaultConfig(), metrics, nil, nil, recorder)
	if err == nil {
		t.Fatalf("expected error, got controller: %+v", controller)
	}

	if !errors.Is(err, errDutyCyclerRequired) {
		t.Fatalf("expected errDutyCyclerRequired, got %v", err)
	}

	if recorderCalls(recorder) != 0 {
		t.Fatalf("recorder should not be called when duty cycler is nil")
	}
}

func TestNewAdaptiveControllerRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.TargetMin = cfg.TargetMax

	metrics := newFakeMetrics(
		[]metricResult{{value: 0.2, timestamp: time.Unix(1_700_000_480, 0), err: nil}},
	)

	controller, err := NewAdaptiveController(cfg, metrics, nil, newFakeShaper(), nil)
	if err == nil {
		t.Fatalf("expected validation error, got controller: %+v", controller)
	}

	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestNewAdaptiveControllerMissingDependenciesWithNilRecorder(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()

	controller, err := NewAdaptiveController(cfg, nil, nil, newFakeShaper(), nil)
	if err == nil {
		t.Fatalf("expected metrics error, got controller: %+v", controller)
	}

	if !errors.Is(err, errMetricsClientRequired) {
		t.Fatalf("expected errMetricsClientRequired, got %v", err)
	}

	metrics := newFakeMetrics(
		[]metricResult{{value: 0.2, timestamp: time.Unix(1_700_000_540, 0), err: nil}},
	)

	controller, err = NewAdaptiveController(cfg, metrics, nil, nil, nil)
	if err == nil {
		t.Fatalf("expected duty cycler error, got controller: %+v", controller)
	}

	if !errors.Is(err, errDutyCyclerRequired) {
		t.Fatalf("expected errDutyCyclerRequired, got %v", err)
	}
}

func recorderCalls(recorder *stubMetricsRecorder) int {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()

	return recorder.modeCalls + recorder.stateCalls + recorder.targetCalls + recorder.ociCalls +
		recorder.hostCalls + recorder.intervalSet + recorder.errorCalls + recorder.relaxedCalls
}

func TestNewAdaptiveControllerHandlesNilRecorder(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics(
		[]metricResult{{value: 0.2, timestamp: time.Unix(1_700_000_360, 0), err: nil}},
	)
	shaper := newFakeShaper()

	controller, err := NewAdaptiveController(DefaultConfig(), metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	if controller == nil {
		t.Fatalf("expected controller, got nil")
	}

	requireFloatApprox(t, "target", controller.Target(), DefaultConfig().FallbackTarget)
	requireFloatApprox(t, "shaperTarget", shaper.Target(), DefaultConfig().FallbackTarget)
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

func TestNewAdaptiveControllerEmbedsEstimatorAndPool(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics(
		[]metricResult{{value: 0.3, timestamp: time.Unix(1_700_000_600, 0), err: nil}},
	)
	sampler := new(fakeEstimator)
	pool := newFakeShaper()

	controller, err := NewAdaptiveController(DefaultConfig(), metrics, sampler, pool, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	if controller.estimator != sampler {
		t.Fatalf("expected estimator to be wired through constructor")
	}

	if controller.shaper != pool {
		t.Fatalf("expected duty cycler to wrap provided pool")
	}
}
