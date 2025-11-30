// Package adapt collects the controller loop tests to keep lifecycle and
// metrics coverage alongside the production orchestration entry point.
//
//nolint:testpackage,godoclint // Tests need internal helpers and per-file coverage documentation.
package adapt

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/est"
)

func TestAdaptiveControllerRunLifecycle(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{
		{value: 0.24, timestamp: time.Unix(1_700_000_100, 0), err: nil},
		{value: 0.26, timestamp: time.Unix(1_700_000_160, 0), err: nil},
	})
	shaper := newFakeShaper()
	cfg := DefaultConfig()
	cfg.Interval = 5 * time.Millisecond
	cfg.RelaxedInterval = 10 * time.Millisecond
	cfg.Mode = "  enforce  "
	cfg.ResourceID = "resource"

	estimator := &fakeEstimator{ //nolint:exhaustruct // consumed counter zero-value is sufficient for tests.
		observations: []est.Observation{
			{
				Timestamp:    time.Unix(0, 0),
				Utilisation:  0.5,
				Runnable:     0,
				BusyJiffies:  0,
				TotalJiffies: 0,
				Err:          nil,
			},
		},
	}

	controller, err := NewAdaptiveController(cfg, metrics, estimator, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)

	go func() {
		done <- controller.Run(ctx)
	}()

	waitFor(func() bool {
		return controller.LastP95() != 0
	}, 500*time.Millisecond)
	cancel()

	err = <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error: %v", err)
	}

	if controller.Mode() != enforceMode {
		t.Fatalf("unexpected mode: %q", controller.Mode())
	}

	if controller.LastP95() == 0 {
		t.Fatalf("expected last p95 to be recorded")
	}

	if estimator.consumed.Load() == 0 {
		t.Fatalf("expected estimator observations to be consumed")
	}
}

func TestAdaptiveControllerRunStepsImmediately(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics(
		[]metricResult{{value: 0.24, timestamp: time.Unix(1_700_000_220, 0), err: nil}},
	)
	shaper := newFakeShaper()
	cfg := DefaultConfig()
	cfg.Interval = 250 * time.Millisecond
	cfg.RelaxedInterval = 500 * time.Millisecond
	cfg.Mode = "enforce"

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)

	go func() {
		done <- controller.Run(ctx)
	}()

	waitFor(func() bool {
		return metrics.CallCount() > 0
	}, 100*time.Millisecond)

	if metrics.CallCount() == 0 {
		t.Fatalf("expected controller to step immediately on startup")
	}

	if metrics.CallCount() > 1 {
		t.Fatalf("expected only the initial step before cancellation, got %d", metrics.CallCount())
	}

	cancel()

	runErr := <-done

	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("Run error: %v", runErr)
	}
}

func TestConsumeEstimatorStopsOnClose(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics(
		[]metricResult{{value: 0.25, timestamp: time.Unix(1_700_000_240, 0), err: nil}},
	)
	shaper := newFakeShaper()
	cfg := DefaultConfig()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	observations := make(chan est.Observation, 1)
	done := make(chan struct{})

	go func() {
		controller.consumeEstimator(context.Background(), observations)
		close(done)
	}()

	observations <- est.Observation{
		Timestamp:    time.Unix(0, 0),
		Utilisation:  0.5,
		Runnable:     0,
		BusyJiffies:  0,
		TotalJiffies: 0,
		Err:          nil,
	}

	close(observations)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("consumeEstimator did not exit after channel close")
	}

	if len(shaper.HostSignals()) == 0 {
		t.Fatal("expected host load to be observed after consuming estimator values")
	}
}

func TestAdaptiveControllerRunHandlesNilEstimatorChannel(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics(
		[]metricResult{{value: 0.25, timestamp: time.Unix(1_700_000_300, 0), err: nil}},
	)
	shaper := newFakeShaper()
	cfg := DefaultConfig()

	estimator := channelEstimator{ch: nil}

	controller, err := NewAdaptiveController(cfg, metrics, estimator, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	ctx := t.Context()

	done := make(chan error, 1)

	go func() {
		done <- controller.Run(ctx)
	}()

	select {
	case runErr := <-done:
		if runErr == nil {
			t.Fatal("expected error from run with nil estimator channel")
		}

		if !errors.Is(runErr, errEstimatorNilChannel) {
			t.Fatalf("unexpected run error: %v", runErr)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not terminate after estimator returned nil channel")
	}
}

func TestConsumeEstimatorStopsWhenEstimatorClosesImmediately(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics(
		[]metricResult{{value: 0.25, timestamp: time.Unix(1_700_000_360, 0), err: nil}},
	)
	shaper := newFakeShaper()
	cfg := DefaultConfig()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	observations := make(chan est.Observation)

	ctx := t.Context()

	var waitGroup sync.WaitGroup

	waitGroup.Go(func() {
		controller.consumeEstimator(ctx, observations)
	})

	close(observations)

	done := make(chan struct{})

	go func() {
		waitGroup.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("consumeEstimator goroutine did not exit after immediate channel close")
	}
}

func TestAdaptiveControllerRunFallsBackOnNegativeIntervals(t *testing.T) {
	t.Parallel()

	controller, metrics, estimatorCh := buildNegativeIntervalController(t)

	ctx := t.Context()

	consumeEstimatorOnce(ctx, t, controller, estimatorCh)

	firstInterval := stepWithFallback(ctx, controller)

	if firstInterval != controller.cfg.Interval {
		t.Fatalf(
			"expected initial fallback interval %v, got %v",
			controller.cfg.Interval,
			firstInterval,
		)
	}

	controller.mu.Lock()
	controller.interval = 2 * controller.cfg.Interval
	controller.mu.Unlock()

	nextInterval := stepWithFallback(ctx, controller)

	controller.mu.Lock()
	resetNeeded := nextInterval != controller.interval
	controller.interval = nextInterval
	controller.mu.Unlock()

	if !resetNeeded {
		t.Fatal("expected ticker reset when interval changed after fallback")
	}

	if controller.interval != controller.cfg.Interval {
		t.Fatalf(
			"expected fallback interval %v after negative relaxed interval, got %v",
			controller.cfg.Interval,
			controller.interval,
		)
	}

	if metrics.CallCount() != 2 {
		t.Fatalf("expected two step invocations, got %d", metrics.CallCount())
	}
}

func TestConsumeEstimatorStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics(
		[]metricResult{{value: 0.25, timestamp: time.Unix(1_700_000_600, 0), err: nil}},
	)
	shaper := newFakeShaper()
	cfg := DefaultConfig()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	observations := make(chan est.Observation)
	done := make(chan struct{})

	go func() {
		controller.consumeEstimator(ctx, observations)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("consumeEstimator did not exit after context cancellation")
	}
}

type channelEstimator struct {
	ch <-chan est.Observation
}

func (c channelEstimator) Run(context.Context) <-chan est.Observation {
	return c.ch
}

func defaultObservation(utilisation float64) est.Observation {
	return est.Observation{
		Timestamp:    time.Unix(0, 0),
		Utilisation:  utilisation,
		Runnable:     0,
		BusyJiffies:  0,
		TotalJiffies: 0,
		Err:          nil,
	}
}

func buildNegativeIntervalController(
	t *testing.T,
) (*AdaptiveController, *fakeMetrics, chan est.Observation) {
	t.Helper()

	metrics := newFakeMetrics([]metricResult{
		{value: 0.95, timestamp: time.Unix(1_700_000_480, 0), err: nil},
		{value: 0.95, timestamp: time.Unix(1_700_000_540, 0), err: nil},
	})
	shaper := newFakeShaper()
	cfg := DefaultConfig()
	cfg.Interval = 50 * time.Millisecond
	cfg.RelaxedInterval = -25 * time.Millisecond
	cfg.RelaxedThreshold = 0.1
	cfg.RelaxedConfirmations = 1

	estimatorCh := make(chan est.Observation, 1)
	estimatorCh <- defaultObservation(0.5)

	close(estimatorCh)

	controller, err := NewAdaptiveController(
		cfg,
		metrics,
		channelEstimator{ch: estimatorCh},
		shaper,
		nil,
	)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	controller.cfg.RelaxedInterval = -25 * time.Millisecond
	controller.cfg.RelaxedThreshold = 0.1
	controller.cfg.RelaxedConfirmations = 1

	return controller, metrics, estimatorCh
}

func consumeEstimatorOnce(
	ctx context.Context,
	t *testing.T,
	controller *AdaptiveController,
	estimatorCh <-chan est.Observation,
) {
	t.Helper()

	consumed := make(chan struct{})

	go func() {
		controller.consumeEstimator(ctx, estimatorCh)
		close(consumed)
	}()

	select {
	case <-consumed:
	case <-time.After(time.Second):
		t.Fatal("expected estimator consumption to finish")
	}
}

func stepWithFallback(ctx context.Context, controller *AdaptiveController) time.Duration {
	interval := controller.step(ctx)
	if interval <= 0 {
		return controller.cfg.Interval
	}

	return interval
}

func TestAdaptiveControllerRunEmitsMetricsSignals(t *testing.T) {
	t.Parallel()

	recorder := newStubMetricsRecorder()
	metrics := newFakeMetrics(
		[]metricResult{{value: 0.20, timestamp: time.Unix(1_700_000_420, 0), err: nil}},
	)
	shaper := newFakeShaper()
	cfg := DefaultConfig()
	cfg.Mode = "  enforce  "

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, recorder)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	requirePositiveInt(t, "modeCalls", recorder.modeCalls)
	requireEqual(t, "mode", recorder.mode, enforceMode)
	requireEqual(t, "initialState", recorder.state, StateFallback.String())
	requireFloatApprox(t, "initialTarget", recorder.target, cfg.FallbackTarget)
	requireEqual(t, "initialInterval", recorder.interval, cfg.Interval)
	requirePositiveInt(t, "initialIntervalCalls", recorder.intervalSet)
	requireEqual(t, "initialLastError", recorder.lastError, nil)
	requirePositiveInt(t, "initialErrorCalls", recorder.errorCalls)

	feedObservation(controller, 0, 0.75, nil)

	requirePositiveInt(t, "hostCalls", recorder.hostCalls)
	requireFloatApprox(t, "hostUtilisation", recorder.host, 0.75)

	stepper, ok := any(controller).(controllerStepper)
	if !ok {
		t.Fatalf("controller does not expose stepper interface")
	}

	stepper.step(context.Background())

	requirePositiveInt(t, "ociCalls", recorder.ociCalls)
	requireFloatApprox(t, "ociValue", recorder.ociValue, 0.20)
	requireNotZeroTime(t, "ociTime", recorder.ociTime)
	requireEqual(t, "stateAfterStep", recorder.state, StateNormal.String())
	requireFloatApprox(t, "targetAfterStep", recorder.target, shaper.Target())
	requireEqual(t, "intervalAfterStep", recorder.interval, cfg.Interval)
	requirePositiveInt(t, "intervalCallsAfterStep", recorder.intervalSet)
	requireEqual(t, "lastErrorAfterStep", recorder.lastError, nil)
	requireTrue(t, "errorCallsAfterStep", recorder.errorCalls >= 2)
}

func TestAdaptiveControllerGuardedSuppressionResetsRelaxedSuccesses(t *testing.T) {
	t.Parallel()

	recorder := newStubMetricsRecorder()
	metrics := newFakeMetrics(
		[]metricResult{
			{value: defaultRelaxedThresh, timestamp: time.Unix(1_700_002_100, 0), err: nil},
		},
	)
	shaper := newFakeShaper()
	cfg := DefaultConfig()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, recorder)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	stepper, ok := any(controller).(controllerStepper)
	if !ok {
		t.Fatal("controller does not expose stepper interface")
	}

	_ = stepper.step(context.Background())

	_, callsBefore := recorder.relaxedSuccesses()

	controller.handleObservation(est.Observation{
		Timestamp:    time.Unix(1_700_002_160, 0),
		Utilisation:  cfg.SuppressThreshold + 0.05,
		Runnable:     0,
		BusyJiffies:  0,
		TotalJiffies: 0,
		Err:          nil,
	})

	if controller.RelaxedSuccesses() != 0 {
		t.Fatalf(
			"expected relaxedSuccesses to reset after guarded suppression, got %d",
			controller.RelaxedSuccesses(),
		)
	}

	relaxed, callsAfter := recorder.relaxedSuccesses()
	requireEqual(t, "recorder relaxed successes after guard", relaxed, 0)
	requireTrue(t, "recorder updated on guarded transition", callsAfter > callsBefore)
}

func TestAdaptiveControllerSmoothedSuppressionTransitionsResetRelaxedSuccesses(t *testing.T) {
	t.Parallel()

	recorder := newStubMetricsRecorder()
	metrics := newFakeMetrics(
		[]metricResult{
			{value: defaultRelaxedThresh, timestamp: time.Unix(1_700_002_220, 0), err: nil},
		},
	)
	shaper := newFakeShaper()
	cfg := DefaultConfig()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, recorder)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	stepper, ok := any(controller).(controllerStepper)
	if !ok {
		t.Fatal("controller does not expose stepper interface")
	}

	_ = stepper.step(context.Background())

	controller.handleObservation(est.Observation{
		Timestamp:    time.Unix(1_700_002_280, 0),
		Utilisation:  cfg.SuppressThreshold + 0.1,
		Runnable:     0,
		BusyJiffies:  0,
		TotalJiffies: 0,
		Err:          nil,
	})

	controller.mu.Lock()
	controller.relaxedSuccesses = 3
	controller.mu.Unlock()

	_, callsBefore := recorder.relaxedSuccesses()

	resumeStart := time.Unix(1_700_002_340, 0)
	if !resumeSuppression(controller, cfg, resumeStart) {
		t.Fatal("expected controller to resume after cooled observations")
	}

	if controller.RelaxedSuccesses() != 0 {
		t.Fatalf(
			"expected relaxedSuccesses to reset after smoothed transition, got %d",
			controller.RelaxedSuccesses(),
		)
	}

	relaxed, callsAfter := recorder.relaxedSuccesses()
	requireEqual(t, "recorder relaxed successes after smoothed transition", relaxed, 0)
	requireTrue(t, "recorder updated on smoothed transition", callsAfter > callsBefore)
}

func resumeSuppression(controller *AdaptiveController, cfg Config, start time.Time) bool {
	for index := range 20 {
		controller.handleObservation(est.Observation{
			Timestamp:    start.Add(time.Duration(index*10) * time.Second),
			Utilisation:  cfg.SuppressResume - 0.05,
			Runnable:     0,
			BusyJiffies:  0,
			TotalJiffies: 0,
			Err:          nil,
		})

		controller.mu.Lock()
		suppressed := controller.suppressed
		controller.mu.Unlock()

		if !suppressed {
			return true
		}
	}

	return false
}

func waitFor(cond func() bool, timeout time.Duration) {
	deadline := time.Now().Add(timeout)

	for {
		if cond() {
			return
		}

		if time.Now().After(deadline) {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}
}
