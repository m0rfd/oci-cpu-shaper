//nolint:testpackage // shared stubs target unexported interfaces for white-box coverage.
package metricsclient

import (
	"context"
	"time"
)

type stubMetricsAdapter struct{}

func (s *stubMetricsAdapter) QueryP95CPU(context.Context, string) (float64, time.Time, error) {
	return 0, time.Time{}, nil
}

type stubP95Querier struct {
	value        float64
	err          error
	calls        int
	lastResource string
}

func (s *stubP95Querier) QueryP95CPU(
	_ context.Context,
	resourceID string,
) (float64, time.Time, error) {
	s.calls++
	s.lastResource = resourceID

	if s.err != nil {
		return 0, time.Time{}, s.err
	}

	return s.value, time.Time{}, nil
}

func newStubP95Querier(value float64, err error) *stubP95Querier {
	return &stubP95Querier{value: value, err: err} //nolint:exhaustruct
}
