package adapt

import (
	"context"
	"time"
)

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

	c.relaxedSuccesses = 0
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

	nextInterval := c.nextIntervalLocked(p95)

	if c.recorder != nil {
		c.recorder.SetInterval(nextInterval)
	}

	return nextInterval
}

func (c *AdaptiveController) nextIntervalLocked(p95 float64) time.Duration {
	if p95 >= c.cfg.RelaxedThreshold {
		c.relaxedSuccesses++
	} else {
		c.relaxedSuccesses = 0
	}

	if c.relaxedSuccesses >= c.cfg.RelaxedConfirmations {
		return c.cfg.RelaxedInterval
	}

	return c.cfg.Interval
}
