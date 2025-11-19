package adapt

// State returns the current controller state.
func (c *AdaptiveController) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.state
}

// Target returns the shaper target tracked by the controller.
func (c *AdaptiveController) Target() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.target
}

// LastError returns the most recent OCI metrics error encountered by the controller.
func (c *AdaptiveController) LastError() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.lastErr
}

// LastP95 returns the last successful OCI P95 value.
func (c *AdaptiveController) LastP95() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.lastP95
}

// LastEstimatorError returns the last observation error from the fast estimator loop.
func (c *AdaptiveController) LastEstimatorError() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.lastEstErr
}

// Mode returns the configured controller mode label.
func (c *AdaptiveController) Mode() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.mode
}

func (c *AdaptiveController) applyTargetLocked(target float64) {
	c.target = target
	c.shaper.SetTarget(target)

	if c.recorder != nil {
		c.recorder.SetTarget(target)
	}
}

func (c *AdaptiveController) updateEffectiveStateLocked() {
	if c.suppressed {
		c.state = StateSuppressed
		if c.recorder != nil {
			c.recorder.SetState(c.state.String())
		}

		return
	}

	c.state = c.slowState
	if c.recorder != nil {
		c.recorder.SetState(c.state.String())
	}
}

func clamp(value, lower, upper float64) float64 {
	if value < lower {
		return lower
	}

	if value > upper {
		return upper
	}

	return value
}
