package adapt

import (
	"context"
	"time"

	"oci-cpu-shaper/pkg/est"
)

// State captures the controller operating mode.
type State int

const (
	// StateNormal represents steady-state operation using OCI feedback.
	StateNormal State = iota
	// StateFallback is entered when OCI metrics are unavailable.
	StateFallback
	// StateSuppressed is entered when the fast estimator detects host contention.
	StateSuppressed
)

// String implements fmt.Stringer for State values.
func (s State) String() string {
	switch s {
	case StateNormal:
		return "normal"
	case StateFallback:
		return "fallback"
	case StateSuppressed:
		return "suppressed"
	default:
		return "unknown"
	}
}

// Controller represents the adaptive control loop surface.
type Controller interface {
	Run(ctx context.Context) error
	Mode() string
	State() State
	LastError() error
	LastEstimatorError() error
}

// DutyCycler is implemented by the shape worker pool.
type DutyCycler interface {
	SetTarget(target float64)
	Target() float64
	ObserveHostLoad(utilisation, runnable float64)
}

// MetricsRecorder captures controller observability signals.
type MetricsRecorder interface {
	SetMode(mode string)
	SetState(state string)
	SetTarget(target float64)
	ObserveOCIP95(value float64, fetchedAt time.Time)
	ObserveHostCPU(utilisation float64)
	SetInterval(interval time.Duration)
	SetLastError(err error)
	SetRelaxedSuccesses(count int)
}

// Estimator exposes the observation stream produced by pkg/est.
type Estimator interface {
	Run(ctx context.Context) <-chan est.Observation
}
