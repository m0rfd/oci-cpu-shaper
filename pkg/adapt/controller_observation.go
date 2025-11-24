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

	runnable := observation.Runnable
	if math.IsNaN(runnable) || math.IsInf(runnable, 0) {
		runnable = 0
	}

	if runnable < 0 {
		runnable = 0
	}

	if c.recorder != nil {
		c.recorder.ObserveHostCPU(utilisation)
	}

	if c.shaper != nil {
		c.shaper.ObserveHostLoad(utilisation)
	}

	if c.cfg.SuppressThreshold <= 0 && c.cfg.SuppressRunnableThreshold <= 0 {
		return
	}

	c.updateHostLoadLocked(utilisation)
	c.hostRunnable = runnable
	previouslySuppressed := c.transitionSuppressionLocked()
	c.applySuppressionTargetsLocked(previouslySuppressed)
	c.updateEffectiveStateLocked()
}
