// Package adapt collects the controller run-loop tests to keep lifecycle and
// metrics coverage alongside the production orchestration entry point.
//
//nolint:testpackage,godoclint // Tests need internal helpers and per-file coverage documentation.
package adapt

import (
	"context"
	"errors"
	"math"
	"sync"
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

	if controller.Mode() != "enforce" {
		t.Fatalf("unexpected mode: %q", controller.Mode())
	}

	if controller.LastP95() == 0 {
		t.Fatalf("expected last p95 to be recorded")
	}

	if estimator.consumed.Load() == 0 {
		t.Fatalf("expected estimator observations to be consumed")
	}
}

func TestAdaptiveControllerEmitsMetricsSignals(t *testing.T) {
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
	requireEqual(t, "mode", recorder.mode, "enforce")
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

func TestAdaptiveControllerDryRunRecordsTargets(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.20, err: nil}})
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

	recorder.ObserveHostLoad(0.85)

	if len(shaper.calls) != 0 {
		t.Fatalf(
			"expected dry-run recorder to ignore host load observations, got %d calls",
			len(shaper.calls),
		)
	}
}

func TestAdaptiveControllerEnforceModeMutatesDutyCycler(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.20, err: nil}})
	shaper := newFakeShaper()
	cfg := DefaultConfig()
	cfg.Mode = "enforce"
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
	nilRecorder.ObserveHostLoad(0.7)

	recorder := &recordingDutyCycler{mu: sync.Mutex{}, target: 0}
	recorder.ObserveHostLoad(0.9)
}

type stubMetricsRecorder struct {
	mu          sync.Mutex
	mode        string
	modeCalls   int
	state       string
	stateCalls  int
	target      float64
	targetCalls int
	ociValue    float64
	ociTime     time.Time
	ociCalls    int
	host        float64
	hostCalls   int
	interval    time.Duration
	intervalSet int
	lastError   error
	errorCalls  int
}

func newStubMetricsRecorder() *stubMetricsRecorder { return new(stubMetricsRecorder) }

func (s *stubMetricsRecorder) SetMode(mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.mode = mode
	s.modeCalls++
}

func (s *stubMetricsRecorder) SetState(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = state
	s.stateCalls++
}

func (s *stubMetricsRecorder) SetTarget(target float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.target = target
	s.targetCalls++
}

func (s *stubMetricsRecorder) ObserveOCIP95(value float64, fetchedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ociValue = value
	s.ociTime = fetchedAt
	s.ociCalls++
}

func (s *stubMetricsRecorder) ObserveHostCPU(utilisation float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.host = utilisation
	s.hostCalls++
}

func (s *stubMetricsRecorder) SetInterval(interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.interval = interval
	s.intervalSet++
}

func (s *stubMetricsRecorder) SetLastError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastError = err
	s.errorCalls++
}

func requireEqual[T comparable](t *testing.T, name string, got, want T) {
	t.Helper()

	if got != want {
		t.Fatalf("expected %s to be %v, got %v", name, want, got)
	}
}

func requireFloatApprox(t *testing.T, name string, got, want float64) {
	t.Helper()

	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("expected %s %.6f, got %.6f", name, want, got)
	}
}

func requirePositiveInt(t *testing.T, name string, value int) {
	t.Helper()

	if value <= 0 {
		t.Fatalf("expected %s to be positive, got %d", name, value)
	}
}

func requireTrue(t *testing.T, name string, condition bool) {
	t.Helper()

	if !condition {
		t.Fatalf("expected %s to be true", name)
	}
}

func requireNotZeroTime(t *testing.T, name string, value time.Time) {
	t.Helper()

	if value.IsZero() {
		t.Fatalf("expected %s to be non-zero", name)
	}
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
