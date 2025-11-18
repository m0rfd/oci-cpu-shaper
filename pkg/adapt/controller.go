package adapt

import (
	"context"
	"fmt"
	"sync"
	"time"

	"oci-cpu-shaper/pkg/est"
	"oci-cpu-shaper/pkg/oci"
)

type recordingDutyCycler struct {
	mu     sync.Mutex
	target float64
}

//nolint:ireturn // callers require the DutyCycler interface seam for testing.
func newModeAwareDutyCycler(mode string, shaper DutyCycler) DutyCycler {
	if shaper == nil {
		return nil
	}

	if ModeEnforcesTargets(mode) {
		return shaper
	}

	recorder := &recordingDutyCycler{
		mu:     sync.Mutex{},
		target: shaper.Target(),
	}

	return recorder
}

func (r *recordingDutyCycler) SetTarget(target float64) {
	if r == nil {
		return
	}

	r.mu.Lock()
	r.target = target
	r.mu.Unlock()
}

func (r *recordingDutyCycler) Target() float64 {
	if r == nil {
		return 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.target
}

func (r *recordingDutyCycler) ObserveHostLoad(float64) {
	if r == nil {
		return
	}

	// dry-run wrapper intentionally ignores host load updates
}

// AdaptiveController orchestrates the normal/fallback state machine.
type AdaptiveController struct {
	cfg       Config
	metrics   oci.MetricsClient
	shaper    DutyCycler
	estimator Estimator
	recorder  MetricsRecorder

	mu         sync.Mutex
	state      State
	slowState  State
	suppressed bool
	target     float64
	desired    float64
	lastP95    float64
	lastErr    error
	lastEstErr error
	hostLoad   float64
	interval   time.Duration
	mode       string
}

var _ Controller = (*AdaptiveController)(nil)

// NewAdaptiveController wires together the OCI metrics client, estimator and shaper.
func NewAdaptiveController(
	cfg Config,
	metrics oci.MetricsClient,
	estimator Estimator,
	shaper DutyCycler,
	recorder MetricsRecorder,
) (*AdaptiveController, error) {
	if metrics == nil {
		return nil, errMetricsClientRequired
	}

	if shaper == nil {
		return nil, errDutyCyclerRequired
	}

	normalized, mode, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	wrappedShaper := newModeAwareDutyCycler(mode, shaper)

	controller := new(AdaptiveController)
	controller.cfg = normalized
	controller.metrics = metrics
	controller.shaper = wrappedShaper
	controller.estimator = estimator
	controller.recorder = recorder
	controller.state = StateFallback
	controller.slowState = StateFallback
	controller.target = normalized.FallbackTarget
	controller.desired = normalized.FallbackTarget
	controller.interval = normalized.Interval
	controller.mode = mode

	wrappedShaper.SetTarget(normalized.FallbackTarget)

	if recorder != nil {
		recorder.SetMode(mode)
		recorder.SetState(controller.state.String())
		recorder.SetTarget(controller.target)
		recorder.SetInterval(controller.interval)
		recorder.SetLastError(nil)
	}

	return controller, nil
}

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
	if p95 <= c.cfg.RelaxedThreshold {
		nextInterval = c.cfg.RelaxedInterval
	}

	if c.recorder != nil {
		c.recorder.SetInterval(nextInterval)
	}

	return nextInterval
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
