//nolint:testpackage // Helpers interact with private controller fields for tests.
package adapt

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"oci-cpu-shaper/pkg/est"
)

var (
	errNoResultsConfigured  = errors.New("test: no results configured")
	errOCIDown              = errors.New("test: oci down")
	errEstimatorObservation = errors.New("test: estimator observation failure")
)

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
