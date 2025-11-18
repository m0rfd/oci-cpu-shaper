// Package adapt collects the controller run-loop tests to keep lifecycle and
// metrics coverage alongside the production orchestration entry point.
//
//nolint:testpackage,godoclint // Tests need internal helpers and per-file coverage documentation.
package adapt

import (
	"context"
	"errors"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/est"
)

func TestAdaptiveControllerRunLifecycle(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.24, err: nil}, {value: 0.26, err: nil}})
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

func TestAdaptiveControllerRunEmitsMetricsSignals(t *testing.T) {
	t.Parallel()

	recorder := newStubMetricsRecorder()
	metrics := newFakeMetrics([]metricResult{{value: 0.20, err: nil}})
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
