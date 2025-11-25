package adapt

import (
	"math"

	"oci-cpu-shaper/pkg/est"
)

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
	runnable := normalizeRunnable(observation.Runnable)

	guarded := c.guardExceeded(utilisation, runnable)

	if c.recorder != nil {
		c.recorder.ObserveHostCPU(utilisation)
	}

	if c.shaper != nil {
		c.shaper.ObserveHostLoad(utilisation, runnable)
	}

	// Optimization: if suppression is disabled, we don't need to track
	// internal state related to suppression transitions beyond the guard path.
	if c.suppressionDisabled() {
		c.handleGuardedSuppressionLocked(guarded)

		return
	}

	c.updateHostLoadLocked(utilisation, guarded)
	c.hostRunnable = runnable
	c.handleSuppressionTransitionLocked(guarded)
}

func (c *AdaptiveController) guardExceeded(utilisation, runnable float64) bool {
	return (c.cfg.SuppressThreshold > 0 && utilisation >= c.cfg.SuppressThreshold) ||
		(c.cfg.SuppressRunnableThreshold > 0 && runnable >= c.cfg.SuppressRunnableThreshold)
}

func normalizeRunnable(runnable float64) float64 {
	if math.IsNaN(runnable) || math.IsInf(runnable, 0) {
		return 0
	}

	if runnable < 0 {
		return 0
	}

	return runnable
}

func (c *AdaptiveController) suppressionDisabled() bool {
	return c.cfg.SuppressThreshold <= 0 && c.cfg.SuppressRunnableThreshold <= 0
}

func (c *AdaptiveController) handleGuardedSuppressionLocked(guarded bool) {
	if !guarded {
		return
	}

	c.suppressed = true
	c.applySuppressionTargetsLocked(false)
	c.updateEffectiveStateLocked()
}

func (c *AdaptiveController) handleSuppressionTransitionLocked(guarded bool) {
	previouslySuppressed := c.transitionSuppressionLocked(guarded)

	if previouslySuppressed != c.suppressed {
		c.resetRelaxedSuccessesLocked()
	}

	c.applySuppressionTargetsLocked(previouslySuppressed)
	c.updateEffectiveStateLocked()
}
