package adapt

const hostLoadSmoothing = 5

func (c *AdaptiveController) updateHostLoadLocked(utilisation float64) {
	if c.hostLoad == 0 {
		c.hostLoad = utilisation

		return
	}

	c.hostLoad += (utilisation - c.hostLoad) / float64(hostLoadSmoothing)
}

func (c *AdaptiveController) transitionSuppressionLocked() bool {
	previous := c.suppressed

	if !c.suppressed && c.hostLoad >= c.cfg.SuppressThreshold {
		c.suppressed = true
	} else if c.suppressed && c.hostLoad <= c.cfg.SuppressResume {
		c.suppressed = false
	}

	return previous
}

func (c *AdaptiveController) applySuppressionTargetsLocked(previouslySuppressed bool) {
	switch {
	case c.suppressed:
		c.applyTargetLocked(0)
	case previouslySuppressed:
		restore := c.desired
		if restore == 0 {
			restore = c.cfg.TargetStart
		}

		restore = clamp(restore, c.cfg.TargetMin, c.cfg.TargetMax)
		c.applyTargetLocked(restore)
	}
}
