package adapt

import "oci-cpu-shaper/pkg/est"

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
