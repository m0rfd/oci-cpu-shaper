//nolint:testpackage // shared stubs target unexported interfaces for white-box coverage.
package metricsclient

import "context"

type stubMetricsAdapter struct{}

func (s *stubMetricsAdapter) QueryP95CPU(context.Context, string) (float64, error) { return 0, nil }

type stubP95Querier struct {
	value        float32
	err          error
	calls        int
	lastResource string
	lastLast7d   bool
}

func (s *stubP95Querier) QueryP95CPU(
	_ context.Context,
	resourceID string,
	last7d bool,
) (float32, error) {
	s.calls++
	s.lastResource = resourceID
	s.lastLast7d = last7d

	if s.err != nil {
		return 0, s.err
	}

	return s.value, nil
}

func newStubP95Querier(value float32, err error) *stubP95Querier {
	return &stubP95Querier{value: value, err: err} //nolint:exhaustruct
}
