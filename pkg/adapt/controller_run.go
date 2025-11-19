package adapt

import (
	"context"
	"fmt"
	"time"

	"oci-cpu-shaper/pkg/est"
)

// Run executes the control loop until the context is cancelled.
func (c *AdaptiveController) Run(ctx context.Context) error {
	if c.estimator != nil {
		go c.consumeEstimator(ctx, c.estimator.Run(ctx))
	}

	ticker := time.NewTicker(c.interval)
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

func (c *AdaptiveController) handleObservation(observation est.Observation) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if observation.Err != nil {
		c.lastEstErr = observation.Err
		c.updateEffectiveStateLocked()

		return
	}

	c.lastEstErr = nil

	utilisation := clamp(observation.Utilisation, 0, 1)
	if c.recorder != nil {
		c.recorder.ObserveHostCPU(utilisation)
	}

	if c.shaper != nil {
		c.shaper.ObserveHostLoad(utilisation)
	}

	if c.cfg.SuppressThreshold <= 0 {
		return
	}

	c.updateHostLoadLocked(utilisation)
	previouslySuppressed := c.transitionSuppressionLocked()
	c.applySuppressionTargetsLocked(previouslySuppressed)
	c.updateEffectiveStateLocked()
}

func (c *AdaptiveController) step(ctx context.Context) time.Duration {
	p95, err := c.metrics.QueryP95CPU(ctx, c.cfg.ResourceID)

	c.mu.Lock()
	defer c.mu.Unlock()

	if err != nil {
		return c.handleStepErrorLocked(err)
	}

	return c.handleStepSuccessLocked(p95, time.Now())
}

func (c *AdaptiveController) handleStepErrorLocked(err error) time.Duration {
	c.slowState = StateFallback

	c.lastErr = err
	if c.recorder != nil {
		c.recorder.SetLastError(err)
	}

	fallback := clamp(c.cfg.FallbackTarget, c.cfg.TargetMin, c.cfg.TargetMax)

	c.desired = fallback
	if !c.suppressed {
		c.applyTargetLocked(fallback)
	}

	c.updateEffectiveStateLocked()

	interval := c.cfg.Interval
	if c.recorder != nil {
		c.recorder.SetInterval(interval)
	}

	return interval
}

func (c *AdaptiveController) handleStepSuccessLocked(
	p95 float64,
	fetchedAt time.Time,
) time.Duration {
	c.slowState = StateNormal

	c.lastErr = nil
	if c.recorder != nil {
		c.recorder.SetLastError(nil)
	}

	c.lastP95 = p95
	if c.recorder != nil {
		c.recorder.ObserveOCIP95(p95, fetchedAt)
	}

	nextTarget := c.target
	if c.suppressed {
		nextTarget = c.desired
	}

	if nextTarget == 0 {
		nextTarget = c.cfg.TargetStart
	}

	if p95 < c.cfg.GoalLow {
		nextTarget += c.cfg.StepUp
	} else if p95 > c.cfg.GoalHigh {
		nextTarget -= c.cfg.StepDown
	}

	nextTarget = clamp(nextTarget, c.cfg.TargetMin, c.cfg.TargetMax)

	c.desired = nextTarget
	if !c.suppressed {
		c.applyTargetLocked(nextTarget)
	}

	c.updateEffectiveStateLocked()

	nextInterval := c.cfg.Interval
	if p95 >= c.cfg.RelaxedThreshold {
		nextInterval = c.cfg.RelaxedInterval
	}

	if c.recorder != nil {
		c.recorder.SetInterval(nextInterval)
	}

	return nextInterval
}
