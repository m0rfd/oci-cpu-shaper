package adapt

import "fmt"

// ValidateConfig ensures controller thresholds are internally consistent.
func ValidateConfig(cfg Config) error {
	err := validateRunnableThresholds(cfg)
	if err != nil {
		return err
	}

	normalized, _ := coerceConfig(cfg)

	return validateControllerConfig(normalized)
}

func validateControllerConfig(cfg Config) error {
	err := validateTargetBounds(cfg)
	if err != nil {
		return err
	}

	err = validateStepSizes(cfg)
	if err != nil {
		return err
	}

	err = validateGoalBounds(cfg)
	if err != nil {
		return err
	}

	return validateSuppressionThresholds(cfg)
}

func validateTargetBounds(cfg Config) error {
	if cfg.TargetMin >= cfg.TargetMax {
		return fmt.Errorf(
			"%w: controller.targetMin (%.2f) must be less than controller.targetMax (%.2f)",
			ErrInvalidConfig,
			cfg.TargetMin,
			cfg.TargetMax,
		)
	}

	for _, threshold := range []struct {
		name  string
		value float64
	}{
		{"controller.targetStart", cfg.TargetStart},
		{"controller.fallbackTarget", cfg.FallbackTarget},
		{"controller.goalLow", cfg.GoalLow},
		{"controller.goalHigh", cfg.GoalHigh},
	} {
		if threshold.value < cfg.TargetMin || threshold.value > cfg.TargetMax {
			return fmt.Errorf(
				"%w: %s (%.2f) must be between controller.targetMin (%.2f) and controller.targetMax (%.2f)",
				ErrInvalidConfig,
				threshold.name,
				threshold.value,
				cfg.TargetMin,
				cfg.TargetMax,
			)
		}
	}

	return nil
}

func validateStepSizes(cfg Config) error {
	for _, step := range []struct {
		name  string
		value float64
	}{
		{"controller.stepUp", cfg.StepUp},
		{"controller.stepDown", cfg.StepDown},
	} {
		if step.value <= 0 {
			return fmt.Errorf(
				"%w: %s (%.3f) must be greater than zero",
				ErrInvalidConfig,
				step.name,
				step.value,
			)
		}
	}

	return nil
}

func validateGoalBounds(cfg Config) error {
	if cfg.GoalLow >= cfg.GoalHigh {
		return fmt.Errorf(
			"%w: controller.goalLow (%.2f) must be less than controller.goalHigh (%.2f)",
			ErrInvalidConfig,
			cfg.GoalLow,
			cfg.GoalHigh,
		)
	}

	return nil
}

func validateSuppressionThresholds(cfg Config) error {
	thresholds := []struct {
		name  string
		value float64
	}{
		{"controller.targetStart", cfg.TargetStart},
		{"controller.targetMin", cfg.TargetMin},
		{"controller.targetMax", cfg.TargetMax},
		{"controller.fallbackTarget", cfg.FallbackTarget},
		{"controller.goalLow", cfg.GoalLow},
		{"controller.goalHigh", cfg.GoalHigh},
	}

	for _, threshold := range thresholds {
		if cfg.SuppressThreshold > 0 && cfg.SuppressThreshold <= threshold.value {
			return fmt.Errorf(
				"%w: controller.suppressThreshold (%.2f) must be greater than %s (%.2f)",
				ErrInvalidConfig,
				cfg.SuppressThreshold,
				threshold.name,
				threshold.value,
			)
		}

		if cfg.SuppressResume > 0 && cfg.SuppressResume <= threshold.value {
			return fmt.Errorf(
				"%w: controller.suppressResume (%.2f) must be greater than %s (%.2f)",
				ErrInvalidConfig,
				cfg.SuppressResume,
				threshold.name,
				threshold.value,
			)
		}
	}

	return validateRunnableThresholds(cfg)
}

func validateRunnableThresholds(cfg Config) error {
	if cfg.SuppressRunnableThreshold < 0 {
		return fmt.Errorf(
			"%w: controller.suppressRunnableThreshold (%.2f) must be zero or greater",
			ErrInvalidConfig,
			cfg.SuppressRunnableThreshold,
		)
	}

	if cfg.SuppressRunnableResume < 0 {
		return fmt.Errorf(
			"%w: controller.suppressRunnableResume (%.2f) must be zero or greater",
			ErrInvalidConfig,
			cfg.SuppressRunnableResume,
		)
	}

	if cfg.SuppressRunnableThreshold > 0 &&
		cfg.SuppressRunnableResume >= cfg.SuppressRunnableThreshold {
		return fmt.Errorf(
			"%w: controller.suppressRunnableResume (%.2f) must be less than controller.suppressRunnableThreshold (%.2f)",
			ErrInvalidConfig,
			cfg.SuppressRunnableResume,
			cfg.SuppressRunnableThreshold,
		)
	}

	return nil
}
