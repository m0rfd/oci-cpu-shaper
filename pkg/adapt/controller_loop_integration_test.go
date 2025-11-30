// Package adapt exercises integration-style controller loop behaviors to verify
// estimator lifecycle expectations alongside internal helpers.
//
//nolint:testpackage,godoclint // Tests access unexported controller hooks for coverage.
package adapt

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/est"
)

func TestAdaptiveControllerRunWithNilEstimatorChannel(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics(
		[]metricResult{{value: 0.25, timestamp: time.Unix(1_700_001_260, 0), err: nil}},
	)
	shaper := newFakeShaper()
	cfg := DefaultConfig()

	estimator := newNilObservationsEstimator()

	controller, err := NewAdaptiveController(cfg, metrics, estimator, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	done := make(chan error, 1)

	go func() {
		done <- controller.Run(t.Context())
	}()

	select {
	case runErr := <-done:
		if runErr == nil {
			t.Fatal("expected controller run to fail when estimator returns nil channel")
		}

		if !errors.Is(runErr, errEstimatorNilChannel) {
			t.Fatalf("unexpected run error: %v", runErr)
		}

		if !estimator.started.Load() {
			t.Fatal("expected estimator to be invoked")
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not terminate after estimator returned nil channel")
	}
}

func TestConsumeEstimatorStopsAfterCancellationWithClosingEstimator(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics(
		[]metricResult{{value: 0.25, timestamp: time.Unix(1_700_001_320, 0), err: nil}},
	)
	shaper := newFakeShaper()
	cfg := DefaultConfig()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	observations := make(chan est.Observation)

	consumerDone := startEstimatorConsumer(ctx, controller, observations)

	emission := emitAndCloseAfterCancel(ctx, observations, est.Observation{
		Timestamp:    time.Unix(0, 0),
		Utilisation:  0.5,
		Runnable:     0,
		BusyJiffies:  0,
		TotalJiffies: 0,
		Err:          nil,
	})

	awaitSignal(t, emission, "observation was not emitted")
	awaitHostLoad(t, shaper)

	cancel()

	awaitSignal(t, consumerDone, "consumeEstimator did not exit after context cancellation")
}

type nilObservationsEstimator struct {
	started atomic.Bool
}

func newNilObservationsEstimator() *nilObservationsEstimator { return new(nilObservationsEstimator) }

func (n *nilObservationsEstimator) Run(context.Context) <-chan est.Observation {
	n.started.Store(true)

	return nil
}

func startEstimatorConsumer(
	ctx context.Context,
	controller *AdaptiveController,
	observations <-chan est.Observation,
) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		controller.consumeEstimator(ctx, observations)
		close(done)
	}()

	return done
}

func emitAndCloseAfterCancel(
	ctx context.Context,
	observations chan<- est.Observation,
	observation est.Observation,
) <-chan struct{} {
	emitted := make(chan struct{})

	go func() {
		observations <- observation

		close(emitted)

		<-ctx.Done()
		close(observations)
	}()

	return emitted
}

func awaitSignal(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal(msg)
	}
}

func awaitHostLoad(t *testing.T, shaper *fakeShaper) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for len(shaper.HostSignals()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("expected host load to be observed before cancellation")
		}

		time.Sleep(10 * time.Millisecond)
	}
}
