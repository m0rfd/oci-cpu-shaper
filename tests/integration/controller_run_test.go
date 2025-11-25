//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/est"
)

func TestControllerRunReturnsNilEstimatorChannel(t *testing.T) {
	t.Parallel()

	metrics := &scriptedMetricsClient{values: []float64{0.24}}
	shaper := newRecordingShaper()

	cfg := adapt.DefaultConfig()
	cfg.ResourceID = "ocid1.instance.oc1..nil-channel"
	cfg.Interval = 10 * time.Millisecond
	cfg.RelaxedInterval = 10 * time.Millisecond

	controller, err := adapt.NewAdaptiveController(cfg, metrics, nilChannelEstimator{}, shaper, nil)
	if err != nil {
		t.Fatalf("create controller: %v", err)
	}

	runErr := controller.Run(t.Context())
	if runErr == nil {
		t.Fatal("expected Run to fail when estimator returns nil channel")
	}

	if !strings.Contains(runErr.Error(), "nil observations channel") {
		t.Fatalf("unexpected Run error: %v", runErr)
	}
}

func TestControllerRunHandlesClosedEstimatorChannel(t *testing.T) {
	t.Parallel()

	notify := make(chan struct{}, 1)
	metrics := &scriptedMetricsClient{values: []float64{0.23}, notify: notify}
	shaper := newRecordingShaper()

	cfg := adapt.DefaultConfig()
	cfg.ResourceID = "ocid1.instance.oc1..closed-channel"
	cfg.Interval = 15 * time.Millisecond
	cfg.RelaxedInterval = 15 * time.Millisecond

	controller, err := adapt.NewAdaptiveController(cfg, metrics, closedChannelEstimator{}, shaper, nil)
	if err != nil {
		t.Fatalf("create controller: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- controller.Run(ctx)
	}()

	select {
	case <-notify:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("controller did not reach metrics query before timeout")
	}

	cancel()

	runErr := <-errCh
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", runErr)
	}
}

func TestControllerRunResetsTickerForChangingIntervals(t *testing.T) {
	t.Parallel()

	metrics := &scriptedMetricsClient{values: []float64{0.80, 0.80, 0.22, 0.22}}
	shaper := newRecordingShaper()

	cfg := adapt.DefaultConfig()
	cfg.ResourceID = "ocid1.instance.oc1..ticker-reset"
	cfg.Interval = 30 * time.Millisecond
	cfg.RelaxedInterval = 90 * time.Millisecond
	cfg.RelaxedThreshold = 0.50

	controller, err := adapt.NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("create controller: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- controller.Run(ctx)
	}()

	waitForCalls(t, metrics, 4, 2*time.Second)
	cancel()

	if runErr := <-errCh; !errors.Is(runErr, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", runErr)
	}

	callTimes := metrics.times()
	if len(callTimes) < 4 {
		t.Fatalf("expected at least four metrics calls, got %d", len(callTimes))
	}

	firstGap := callTimes[1].Sub(callTimes[0])
	secondGap := callTimes[2].Sub(callTimes[1])
	thirdGap := callTimes[3].Sub(callTimes[2])

	if firstGap < cfg.Interval/2 || firstGap > cfg.Interval*2 {
		t.Fatalf("expected base interval gap near %v, got %v", cfg.Interval, firstGap)
	}

	if secondGap < cfg.RelaxedInterval/2 {
		t.Fatalf("expected relaxed interval gap around %v, got %v", cfg.RelaxedInterval, secondGap)
	}

	if thirdGap < cfg.Interval/2 || thirdGap > cfg.Interval*2 {
		t.Fatalf("expected base interval gap near %v after reset, got %v", cfg.Interval, thirdGap)
	}
}

type scriptedMetricsClient struct {
	mu     sync.Mutex
	values []float64
	notify chan struct{}
	calls  []time.Time
}

func (s *scriptedMetricsClient) QueryP95CPU(context.Context, string) (float64, time.Time, error) {
        s.mu.Lock()
        defer s.mu.Unlock()

        now := time.Now()

        s.calls = append(s.calls, now)
        if s.notify != nil {
                select {
                case s.notify <- struct{}{}:
                default:
                }
	}

	if len(s.values) == 0 {
                return 0.25, now, nil
        }

        value := s.values[0]
        s.values = s.values[1:]

        return value, now, nil
}

func (s *scriptedMetricsClient) times() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()

	times := make([]time.Time, len(s.calls))
	copy(times, s.calls)

	return times
}

type nilChannelEstimator struct{}

type closedChannelEstimator struct{}

func (nilChannelEstimator) Run(context.Context) <-chan est.Observation {
	return nil
}

func (closedChannelEstimator) Run(context.Context) <-chan est.Observation {
	ch := make(chan est.Observation)
	close(ch)

	return ch
}

func waitForCalls(t *testing.T, metrics *scriptedMetricsClient, count int, timeout time.Duration) {
	t.Helper()

	deadline := time.After(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if len(metrics.times()) >= count {
			return
		}

		select {
		case <-deadline:
			t.Fatalf("expected %d metrics calls within %v", count, timeout)
		case <-ticker.C:
		}
	}
}
