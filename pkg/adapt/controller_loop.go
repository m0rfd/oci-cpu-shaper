package adapt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"oci-cpu-shaper/pkg/est"
)

var errEstimatorNilChannel = errors.New("estimator returned nil observations channel")

// Run executes the control loop until the context is cancelled.
func (c *AdaptiveController) Run(ctx context.Context) error {
	if c.estimator != nil {
		estimatorCh := c.estimator.Run(ctx)
		if estimatorCh == nil {
			return fmt.Errorf("adaptive controller run: %w", errEstimatorNilChannel)
		}

		go c.consumeEstimator(ctx, estimatorCh)
	}

	nextInterval := c.step(ctx)
	if nextInterval <= 0 {
		nextInterval = c.cfg.Interval
	}

	c.mu.Lock()
	c.interval = nextInterval
	c.mu.Unlock()

	ticker := time.NewTicker(nextInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			if err != nil {
				return fmt.Errorf("adaptive controller run: %w", err)
			}

			return nil
		case <-ticker.C:
			nextInterval := c.step(ctx)
			if nextInterval <= 0 {
				nextInterval = c.cfg.Interval
			}

			if nextInterval != c.interval {
				ticker.Reset(nextInterval)
			}

			c.mu.Lock()
			c.interval = nextInterval
			c.mu.Unlock()
		}
	}
}

func (c *AdaptiveController) consumeEstimator(ctx context.Context, ch <-chan est.Observation) {
	for {
		select {
		case <-ctx.Done():
			return
		case observation, ok := <-ch:
			if !ok {
				return
			}

			c.handleObservation(observation)
		}
	}
}
