package adapt

const hostLoadSmoothing = 5

func (c *AdaptiveController) updateHostLoadLocked(utilisation float64) {
	if c.hostLoad == 0 {
		c.hostLoad = utilisation

		return
	}

	c.hostLoad += (utilisation - c.hostLoad) / float64(hostLoadSmoothing)
}

func (c *AdaptiveController) transitionSuppressionLocked(guarded bool) bool {
	previous := c.suppressed

	switch {
	case guarded:
		c.suppressed = true
	case !c.suppressed && (c.shouldSuppressForUtilisation() || c.shouldSuppressForRunnables()):
		c.suppressed = true
	case c.suppressed && c.shouldResumeFromSuppression():
		c.suppressed = false
	}

	return previous
}

func (c *AdaptiveController) shouldSuppressForUtilisation() bool {
	return c.cfg.SuppressThreshold > 0 && c.hostLoad >= c.cfg.SuppressThreshold
}

func (c *AdaptiveController) shouldSuppressForRunnables() bool {
	return c.cfg.SuppressRunnableThreshold > 0 && c.hostRunnable >= c.cfg.SuppressRunnableThreshold
}

func (c *AdaptiveController) shouldResumeFromSuppression() bool {
	return c.utilisationCooled() && c.runnablesCooled()
}

func (c *AdaptiveController) utilisationCooled() bool {
	if c.cfg.SuppressThreshold <= 0 {
		return true
	}

	return c.hostLoad <= c.cfg.SuppressResume
}

func (c *AdaptiveController) runnablesCooled() bool {
	if c.cfg.SuppressRunnableThreshold <= 0 {
		return true
	}

	return c.hostRunnable <= c.cfg.SuppressRunnableResume
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
