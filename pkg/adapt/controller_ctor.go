package adapt

import (
	"sync"
	"time"

	"oci-cpu-shaper/pkg/oci"
)

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
