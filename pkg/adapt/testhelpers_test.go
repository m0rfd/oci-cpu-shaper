//nolint:testpackage // Helpers interact with private controller fields for tests.
package adapt

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/est"
)

var (
	errNoResultsConfigured  = errors.New("test: no results configured")
	errOCIDown              = errors.New("test: oci down")
	errEstimatorObservation = errors.New("test: estimator observation failure")
)

const enforceMode = "enforce"

type metricResult struct {
	value float64
	err   error
}

type fakeMetrics struct {
	results   []metricResult
	callIndex int
	mu        sync.Mutex
}

func newFakeMetrics(results []metricResult) *fakeMetrics {
	copied := make([]metricResult, len(results))
	copy(copied, results)

	return &fakeMetrics{results: copied, callIndex: 0, mu: sync.Mutex{}}
}

func (f *fakeMetrics) QueryP95CPU(ctx context.Context, _ string) (float64, error) {
	if len(f.results) == 0 {
		return 0, errNoResultsConfigured
	}

	err := ctx.Err()
	if err != nil {
		return 0, fmt.Errorf("query p95 context: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.callIndex >= len(f.results) {
		last := f.results[len(f.results)-1]

		return last.value, last.err
	}

	result := f.results[f.callIndex]
	f.callIndex++

	return result.value, result.err
}

type fakeShaper struct {
	target    float64
	calls     []float64
	hostLoads []float64
}

func newFakeShaper() *fakeShaper {
	return &fakeShaper{target: 0, calls: make([]float64, 0), hostLoads: make([]float64, 0)}
}

func (f *fakeShaper) SetTarget(v float64) {
	f.target = v
	f.calls = append(f.calls, v)
}

func (f *fakeShaper) Target() float64 { return f.target }

func (f *fakeShaper) ObserveHostLoad(util float64) {
	f.hostLoads = append(f.hostLoads, util)
}

type fakeEstimator struct {
	observations []est.Observation
	consumed     atomic.Int32
}

func (f *fakeEstimator) Run(context.Context) <-chan est.Observation {
	observationsCh := make(chan est.Observation, len(f.observations))
	for _, observation := range f.observations {
		observationsCh <- observation

		f.consumed.Add(1)
	}

	close(observationsCh)

	return observationsCh
}

func feedObservation(controller *AdaptiveController, ts int64, utilisation float64, err error) {
	controller.handleObservation(est.Observation{
		Timestamp:    time.Unix(ts, 0),
		Utilisation:  utilisation,
		BusyJiffies:  0,
		TotalJiffies: 0,
		Err:          err,
	})
}

type controllerStepper interface {
	step(ctx context.Context) time.Duration
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
