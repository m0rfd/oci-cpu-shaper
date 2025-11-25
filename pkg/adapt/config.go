package adapt

import (
	"errors"
	"time"
)

// Config defines controller thresholds.
type Config struct {
	ResourceID       string
	Mode             string
	TargetStart      float64
	TargetMin        float64
	TargetMax        float64
	StepUp           float64
	StepDown         float64
	FallbackTarget   float64
	GoalLow          float64
	GoalHigh         float64
	Interval         time.Duration
	RelaxedInterval  time.Duration
	RelaxedThreshold float64
	// RelaxedConfirmations specifies how many consecutive samples
	// with P95 >= RelaxedThreshold are required before switching
	// from Interval to RelaxedInterval. This prevents interval flapping
	// during transient load spikes. The counter resets to zero whenever
	// P95 drops below the threshold. Must be > 0. Default: 2.
	RelaxedConfirmations      int
	SuppressThreshold         float64
	SuppressResume            float64
	SuppressRunnableThreshold float64
	SuppressRunnableResume    float64
	// SuppressSmoothingSamples controls how aggressively host utilisation samples
	// are smoothed before evaluating suppression. Values <= 1 disable smoothing
	// and apply the latest sample immediately, while higher counts retain the
	// rolling average behaviour used to dampen oscillations.
	SuppressSmoothingSamples int
}

var (
	errMetricsClientRequired = errors.New("adapt: metrics client is required")
	errDutyCyclerRequired    = errors.New("adapt: duty cycler is required")
	// ErrInvalidConfig signals that the supplied controller configuration is invalid.
	ErrInvalidConfig = errors.New("adapt: invalid config")
)
