package runtimeconfig

import (
	"fmt"
	"time"

	"oci-cpu-shaper/pkg/adapt"
)

func validateRuntimeConfig(cfg Config) error {
	err := validatePoolSettings(cfg.Pool)
	if err != nil {
		return err
	}

	err = validateLoopIntervals(cfg.Controller, cfg.Estimator)
	if err != nil {
		return err
	}

	return validateControllerThresholds(cfg.Controller)
}

func validatePoolSettings(pool PoolConfig) error {
	if pool.Workers <= 0 {
		return invalidConfigError("pool.workers (%d) must be greater than zero", pool.Workers)
	}

	return ensurePositiveDuration("pool.quantum", pool.Quantum)
}

func validateLoopIntervals(controller ControllerConfig, estimator EstimatorConfig) error {
	for _, interval := range []struct {
		name  string
		value time.Duration
	}{
		{"controller.interval", controller.Interval},
		{"controller.relaxedInterval", controller.RelaxedInterval},
		{"estimator.interval", estimator.Interval},
	} {
		err := ensurePositiveDuration(interval.name, interval.value)
		if err != nil {
			return err
		}
	}

	for _, step := range []struct {
		name  string
		value float64
	}{
		{"controller.stepUp", controller.StepUp},
		{"controller.stepDown", controller.StepDown},
	} {
		err := ensurePositiveFloat(step.name, step.value)
		if err != nil {
			return err
		}
	}

	return ensurePositiveInt("controller.relaxedConfirmations", controller.RelaxedConfirmations)
}

func ensurePositiveInt(name string, value int) error {
	if value <= 0 {
		return invalidConfigError("%s (%d) must be greater than zero", name, value)
	}

	return nil
}

func validateControllerThresholds(controller ControllerConfig) error {
	if controller.TargetMin >= controller.TargetMax {
		return invalidConfigError(
			"controller.targetMin (%.2f) must be less than controller.targetMax (%.2f)",
			controller.TargetMin,
			controller.TargetMax,
		)
	}

	for _, threshold := range []struct {
		name  string
		value float64
	}{
		{"controller.targetStart", controller.TargetStart},
		{"controller.fallbackTarget", controller.FallbackTarget},
		{"controller.goalLow", controller.GoalLow},
		{"controller.goalHigh", controller.GoalHigh},
	} {
		err := ensureWithinTargetBounds(
			threshold.name,
			threshold.value,
			controller.TargetMin,
			controller.TargetMax,
		)
		if err != nil {
			return err
		}
	}

	if controller.GoalLow >= controller.GoalHigh {
		return invalidConfigError(
			"controller.goalLow (%.2f) must be less than controller.goalHigh (%.2f)",
			controller.GoalLow,
			controller.GoalHigh,
		)
	}

	return nil
}

func ensurePositiveDuration(name string, value time.Duration) error {
	if value <= 0 {
		return invalidConfigError("%s (%s) must be greater than zero", name, value)
	}

	return nil
}

func ensurePositiveFloat(name string, value float64) error {
	if value <= 0 {
		return invalidConfigError("%s (%.2f) must be greater than zero", name, value)
	}

	return nil
}

func ensureWithinTargetBounds(name string, value, lowerBound, upperBound float64) error {
	if value < lowerBound || value > upperBound {
		return invalidConfigError(
			"%s (%.2f) must be between controller.targetMin (%.2f) and controller.targetMax (%.2f)",
			name,
			value,
			lowerBound,
			upperBound,
		)
	}

	return nil
}

func invalidConfigError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", adapt.ErrInvalidConfig, fmt.Sprintf(format, args...))
}
