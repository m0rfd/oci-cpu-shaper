package adapt

import (
	"errors"
	"time"
)

// Config defines controller thresholds.
type Config struct {
	ResourceID                string
	Mode                      string
	TargetStart               float64
	TargetMin                 float64
	TargetMax                 float64
	StepUp                    float64
	StepDown                  float64
	FallbackTarget            float64
	GoalLow                   float64
	GoalHigh                  float64
	Interval                  time.Duration
	RelaxedInterval           time.Duration
	RelaxedThreshold          float64
	RelaxedConfirmations      int
	SuppressThreshold         float64
	SuppressResume            float64
	SuppressRunnableThreshold float64
	SuppressRunnableResume    float64
}

var (
	errMetricsClientRequired = errors.New("adapt: metrics client is required")
	errDutyCyclerRequired    = errors.New("adapt: duty cycler is required")
	// ErrInvalidConfig signals that the supplied controller configuration is invalid.
	ErrInvalidConfig = errors.New("adapt: invalid config")
)
