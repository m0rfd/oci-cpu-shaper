package adapt

import (
	"sync"
	"time"

	"oci-cpu-shaper/pkg/oci"
)

// AdaptiveController orchestrates the normal/fallback state machine.
//
// The controller maintains a relaxedSuccesses counter that tracks consecutive
// samples with P95 >= RelaxedThreshold. This counter is used to implement
// hysteresis when switching from the normal interval (e.g., 1 hour) to the
// relaxed interval (e.g., 4 hours). Once RelaxedConfirmations consecutive
// high samples are observed, the controller switches to the relaxed interval
// to reduce OCI API calls when the system is consistently above threshold.
//
// Restart Behavior:
// The relaxedSuccesses counter is not persisted across controller restarts.
// After a restart, the counter resets to 0, meaning the controller will poll
// at the normal interval for the first RelaxedConfirmations samples before
// switching back to the relaxed interval (if P95 remains high). This is an
// acceptable trade-off that keeps the implementation simple while ensuring
// correct behavior even in the presence of transient restarts.
//
// The counter also resets to 0 whenever:
// - P95 drops below RelaxedThreshold
// - OCI metrics queries fail (entering fallback state)
//
// Observability:
// The current counter value can be accessed via the RelaxedSuccesses() getter
// for debugging and monitoring purposes.
type AdaptiveController struct {
	cfg       Config
	metrics   oci.MetricsClient
	shaper    DutyCycler
	estimator Estimator
	recorder  MetricsRecorder

	mu               sync.Mutex
	state            State
	slowState        State
	suppressed       bool
	target           float64
	desired          float64
	lastP95          float64
	lastErr          error
	lastEstErr       error
	hostLoad         float64
	hostRunnable     float64
	interval         time.Duration
	mode             string
	relaxedSuccesses int
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
	controller.setSlowStateLocked(StateFallback)
	controller.target = normalized.FallbackTarget
	controller.desired = normalized.FallbackTarget
	controller.interval = normalized.Interval
	controller.mode = mode

	wrappedShaper.SetTarget(normalized.FallbackTarget)

	if recorder != nil {
		recorder.SetMode(mode)
		recorder.SetControllerState(controller.slowState.String())
		recorder.SetState(controller.state.String())
		recorder.SetTarget(controller.target)
		recorder.SetInterval(controller.interval)
		recorder.SetLastError(nil)
	}

	return controller, nil
}
