package runtimeconfig

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	envTargetStart               = "SHAPER_TARGET_START"
	envTargetMin                 = "SHAPER_TARGET_MIN"
	envTargetMax                 = "SHAPER_TARGET_MAX"
	envStepUp                    = "SHAPER_STEP_UP"
	envStepDown                  = "SHAPER_STEP_DOWN"
	envSlowInterval              = "SHAPER_SLOW_INTERVAL"
	envRelaxedInterval           = "SHAPER_SLOW_INTERVAL_RELAXED"
	envFastInterval              = "SHAPER_FAST_INTERVAL"
	envPoolWorkers               = "SHAPER_WORKER_COUNT"
	envHTTPBind                  = "HTTP_ADDR"
	envCompartmentID             = "OCI_COMPARTMENT_ID"
	envOCIRegion                 = "OCI_REGION"
	envInstanceID                = "OCI_INSTANCE_ID"
	envOCIOffline                = "OCI_OFFLINE"
	envFallbackTarget            = "SHAPER_FALLBACK_TARGET"
	envRelaxedThreshold          = "SHAPER_RELAXED_THRESHOLD"
	envGoalLow                   = "SHAPER_GOAL_LOW"
	envGoalHigh                  = "SHAPER_GOAL_HIGH"
	envSuppressThreshold         = "SHAPER_SUPPRESS_THRESHOLD"
	envSuppressResume            = "SHAPER_SUPPRESS_RESUME"
	envSuppressRunnableThreshold = "SHAPER_SUPPRESS_RUNNABLE_THRESHOLD"
	envSuppressRunnableResume    = "SHAPER_SUPPRESS_RUNNABLE_RESUME"
	envPoolPauseThreshold        = "SHAPER_POOL_PAUSE_THRESHOLD"
	envPoolResumeThreshold       = "SHAPER_POOL_RESUME_THRESHOLD"
)

var lookupEnv = os.LookupEnv //nolint:gochecknoglobals // overridden in tests

func applyEnvOverrides(cfg *Config) {
	cfg.Controller.TargetStart = envFloat(envTargetStart, cfg.Controller.TargetStart)
	cfg.Controller.TargetMin = envFloat(envTargetMin, cfg.Controller.TargetMin)
	cfg.Controller.TargetMax = envFloat(envTargetMax, cfg.Controller.TargetMax)
	cfg.Controller.StepUp = envFloat(envStepUp, cfg.Controller.StepUp)
	cfg.Controller.StepDown = envFloat(envStepDown, cfg.Controller.StepDown)
	cfg.Controller.FallbackTarget = envFloat(envFallbackTarget, cfg.Controller.FallbackTarget)
	cfg.Controller.GoalLow = envFloat(envGoalLow, cfg.Controller.GoalLow)
	cfg.Controller.GoalHigh = envFloat(envGoalHigh, cfg.Controller.GoalHigh)
	cfg.Controller.RelaxedThreshold = envFloat(envRelaxedThreshold, cfg.Controller.RelaxedThreshold)
	cfg.Controller.SuppressThreshold = envFloat(
		envSuppressThreshold,
		cfg.Controller.SuppressThreshold,
	)
	cfg.Controller.SuppressResume = envFloat(envSuppressResume, cfg.Controller.SuppressResume)
	cfg.Controller.SuppressRunnableThreshold = envFloat(
		envSuppressRunnableThreshold,
		cfg.Controller.SuppressRunnableThreshold,
	)
	cfg.Controller.SuppressRunnableResume = envFloat(
		envSuppressRunnableResume,
		cfg.Controller.SuppressRunnableResume,
	)
	cfg.Controller.Interval = envDuration(envSlowInterval, cfg.Controller.Interval)
	cfg.Controller.RelaxedInterval = envDuration(envRelaxedInterval, cfg.Controller.RelaxedInterval)

	cfg.Estimator.Interval = envDuration(envFastInterval, cfg.Estimator.Interval)

	cfg.Pool.Workers = envInt(envPoolWorkers, cfg.Pool.Workers)
	cfg.Pool.PauseThreshold = envFloat(envPoolPauseThreshold, cfg.Pool.PauseThreshold)
	cfg.Pool.ResumeThreshold = envFloat(envPoolResumeThreshold, cfg.Pool.ResumeThreshold)

	cfg.HTTP.Bind = envStringAllowEmpty(envHTTPBind, cfg.HTTP.Bind)

	cfg.OCI.CompartmentID = envString(envCompartmentID, cfg.OCI.CompartmentID)
	cfg.OCI.Region = envString(envOCIRegion, cfg.OCI.Region)
	cfg.OCI.InstanceID = envString(envInstanceID, cfg.OCI.InstanceID)
	cfg.OCI.Offline = envBool(envOCIOffline, cfg.OCI.Offline)
}

func envFloat(key string, fallback float64) float64 {
	value, ok := lookupEnv(key)
	if !ok {
		return fallback
	}

	return parseFloatDefault(value, fallback)
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value, ok := lookupEnv(key)
	if !ok {
		return fallback
	}

	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}

	duration, err := time.ParseDuration(trimmed)
	if err != nil {
		return fallback
	}

	return duration
}

func envInt(key string, fallback int) int {
	value, ok := lookupEnv(key)
	if !ok {
		return fallback
	}

	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(trimmed)
	if err != nil || parsed <= 0 {
		return fallback
	}

	return parsed
}

func envString(key, fallback string) string {
	value, ok := lookupEnv(key)
	if !ok {
		return fallback
	}

	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}

	return trimmed
}

func envStringAllowEmpty(key, fallback string) string {
	value, ok := lookupEnv(key)
	if !ok {
		return fallback
	}

	return strings.TrimSpace(value)
}

func envBool(key string, fallback bool) bool {
	value, ok := lookupEnv(key)
	if !ok {
		return fallback
	}

	trimmed := strings.TrimSpace(strings.ToLower(value))
	switch trimmed {
	case "1", "t", "true", "yes", "y":
		return true
	case "0", "f", "false", "no", "n":
		return false
	default:
		return fallback
	}
}

func parseFloatDefault(value string, fallback float64) float64 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}

	parsed, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return fallback
	}

	return parsed
}
