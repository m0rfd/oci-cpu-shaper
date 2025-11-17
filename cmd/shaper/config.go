package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/shape"
)

const (
	envTargetStart       = "SHAPER_TARGET_START"
	envTargetMin         = "SHAPER_TARGET_MIN"
	envTargetMax         = "SHAPER_TARGET_MAX"
	envStepUp            = "SHAPER_STEP_UP"
	envStepDown          = "SHAPER_STEP_DOWN"
	envSlowInterval      = "SHAPER_SLOW_INTERVAL"
	envRelaxedInterval   = "SHAPER_SLOW_INTERVAL_RELAXED"
	envFastInterval      = "SHAPER_FAST_INTERVAL"
	envPoolWorkers       = "SHAPER_WORKER_COUNT"
	envHTTPBind          = "HTTP_ADDR"
	envCompartmentID     = "OCI_COMPARTMENT_ID"
	envOCIRegion         = "OCI_REGION"
	envInstanceID        = "OCI_INSTANCE_ID"
	envOCIOffline        = "OCI_OFFLINE"
	envFallbackTarget    = "SHAPER_FALLBACK_TARGET"
	envRelaxedThreshold  = "SHAPER_RELAXED_THRESHOLD"
	envGoalLow           = "SHAPER_GOAL_LOW"
	envGoalHigh          = "SHAPER_GOAL_HIGH"
	envSuppressThreshold = "SHAPER_SUPPRESS_THRESHOLD"
	envSuppressResume    = "SHAPER_SUPPRESS_RESUME"
)

type runtimeConfig struct {
	Controller controllerConfig
	Estimator  estimatorConfig
	Pool       poolConfig
	HTTP       httpConfig
	OCI        ociConfig
}

type controllerConfig struct {
	TargetStart       float64
	TargetMin         float64
	TargetMax         float64
	StepUp            float64
	StepDown          float64
	FallbackTarget    float64
	GoalLow           float64
	GoalHigh          float64
	Interval          time.Duration
	RelaxedInterval   time.Duration
	RelaxedThreshold  float64
	SuppressThreshold float64
	SuppressResume    float64
}

type estimatorConfig struct {
	Interval time.Duration
}

type poolConfig struct {
	Workers int
	Quantum time.Duration
}

type httpConfig struct {
	Bind string
}

type ociConfig struct {
	CompartmentID string
	Region        string
	InstanceID    string
	Offline       bool
}

type fileConfig struct {
	Controller controllerFileConfig `yaml:"controller"`
	Estimator  estimatorFileConfig  `yaml:"estimator"`
	Pool       poolFileConfig       `yaml:"pool"`
	HTTP       httpFileConfig       `yaml:"http"`
	OCI        ociFileConfig        `yaml:"oci"`
}

type controllerFileConfig struct {
	TargetStart       *float64       `yaml:"targetStart"`
	TargetMin         *float64       `yaml:"targetMin"`
	TargetMax         *float64       `yaml:"targetMax"`
	StepUp            *float64       `yaml:"stepUp"`
	StepDown          *float64       `yaml:"stepDown"`
	FallbackTarget    *float64       `yaml:"fallbackTarget"`
	GoalLow           *float64       `yaml:"goalLow"`
	GoalHigh          *float64       `yaml:"goalHigh"`
	Interval          *time.Duration `yaml:"interval"`
	RelaxedInterval   *time.Duration `yaml:"relaxedInterval"`
	RelaxedThreshold  *float64       `yaml:"relaxedThreshold"`
	SuppressThreshold *float64       `yaml:"suppressThreshold"`
	SuppressResume    *float64       `yaml:"suppressResume"`
}

type estimatorFileConfig struct {
	Interval *time.Duration `yaml:"interval"`
}

type poolFileConfig struct {
	Workers *int           `yaml:"workers"`
	Quantum *time.Duration `yaml:"quantum"`
}

type httpFileConfig struct {
	Bind *string `yaml:"bind"`
}

type ociFileConfig struct {
	CompartmentID *string `yaml:"compartmentId"`
	Region        *string `yaml:"region"`
	InstanceID    *string `yaml:"instanceId"`
	Offline       *bool   `yaml:"offline"`
}

func defaultRuntimeConfig() runtimeConfig {
	defaults := adapt.DefaultConfig()

	var cfg runtimeConfig

	cfg.Controller.TargetStart = defaults.TargetStart
	cfg.Controller.TargetMin = defaults.TargetMin
	cfg.Controller.TargetMax = defaults.TargetMax
	cfg.Controller.StepUp = defaults.StepUp
	cfg.Controller.StepDown = defaults.StepDown
	cfg.Controller.FallbackTarget = defaults.FallbackTarget
	cfg.Controller.GoalLow = defaults.GoalLow
	cfg.Controller.GoalHigh = defaults.GoalHigh
	cfg.Controller.Interval = defaults.Interval
	cfg.Controller.RelaxedInterval = defaults.RelaxedInterval
	cfg.Controller.RelaxedThreshold = defaults.RelaxedThreshold
	cfg.Controller.SuppressThreshold = defaults.SuppressThreshold
	cfg.Controller.SuppressResume = defaults.SuppressResume

	cfg.Estimator.Interval = time.Second

	cfg.Pool.Workers = runtime.NumCPU()
	if cfg.Pool.Workers <= 0 {
		cfg.Pool.Workers = 1
	}

	cfg.Pool.Quantum = shape.DefaultQuantum

	cfg.HTTP.Bind = ":9108"

	return cfg
}

func loadConfig(path string) (runtimeConfig, error) {
	cfg := defaultRuntimeConfig()

	trimmed := strings.TrimSpace(path)
	if trimmed != "" {
		err := mergeRuntimeConfigFile(&cfg, trimmed)
		if err != nil {
			return runtimeConfig{}, err
		}
	}

	applyEnvOverrides(&cfg)

	err := validateRuntimeConfig(cfg)
	if err != nil {
		return runtimeConfig{}, fmt.Errorf("validate runtime config: %w", err)
	}

	err = adapt.ValidateConfig(runtimeToAdaptControllerConfig(cfg))
	if err != nil {
		return runtimeConfig{}, fmt.Errorf("validate controller config: %w", err)
	}

	return cfg, nil
}

func mergeControllerConfig(dst *controllerConfig, src controllerFileConfig) {
	assignFloat(&dst.TargetStart, src.TargetStart)
	assignFloat(&dst.TargetMin, src.TargetMin)
	assignFloat(&dst.TargetMax, src.TargetMax)
	assignFloat(&dst.StepUp, src.StepUp)
	assignFloat(&dst.StepDown, src.StepDown)
	assignFloat(&dst.FallbackTarget, src.FallbackTarget)
	assignFloat(&dst.GoalLow, src.GoalLow)
	assignFloat(&dst.GoalHigh, src.GoalHigh)
	assignDuration(&dst.Interval, src.Interval)
	assignDuration(&dst.RelaxedInterval, src.RelaxedInterval)
	assignFloat(&dst.RelaxedThreshold, src.RelaxedThreshold)
	assignFloat(&dst.SuppressThreshold, src.SuppressThreshold)
	assignFloat(&dst.SuppressResume, src.SuppressResume)
}

func mergeEstimatorConfig(dst *estimatorConfig, src estimatorFileConfig) {
	assignDuration(&dst.Interval, src.Interval)
}

func mergePoolConfig(dst *poolConfig, src poolFileConfig) {
	assignInt(&dst.Workers, src.Workers)
	assignDuration(&dst.Quantum, src.Quantum)
}

func mergeHTTPConfig(dst *httpConfig, src httpFileConfig) {
	assignString(&dst.Bind, src.Bind)
}

func mergeOCIConfig(dst *ociConfig, src ociFileConfig) {
	assignString(&dst.CompartmentID, src.CompartmentID)
	assignString(&dst.Region, src.Region)
	assignString(&dst.InstanceID, src.InstanceID)
	assignBool(&dst.Offline, src.Offline)
}

func applyEnvOverrides(cfg *runtimeConfig) {
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
	cfg.Controller.Interval = envDuration(envSlowInterval, cfg.Controller.Interval)
	cfg.Controller.RelaxedInterval = envDuration(envRelaxedInterval, cfg.Controller.RelaxedInterval)
	cfg.Estimator.Interval = envDuration(envFastInterval, cfg.Estimator.Interval)
	cfg.Pool.Workers = envInt(envPoolWorkers, cfg.Pool.Workers)
	cfg.HTTP.Bind = envString(envHTTPBind, cfg.HTTP.Bind)
	cfg.OCI.CompartmentID = envString(envCompartmentID, cfg.OCI.CompartmentID)
	cfg.OCI.Region = envString(envOCIRegion, cfg.OCI.Region)
	cfg.OCI.InstanceID = envString(envInstanceID, cfg.OCI.InstanceID)
	cfg.OCI.Offline = envBool(envOCIOffline, cfg.OCI.Offline)
}

var lookupEnv = os.LookupEnv //nolint:gochecknoglobals // overridden in tests

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

func assignFloat(target *float64, value *float64) {
	if value != nil {
		*target = *value
	}
}

func assignDuration(target *time.Duration, value *time.Duration) {
	if value != nil {
		*target = *value
	}
}

func assignInt(target *int, value *int) {
	if value != nil {
		*target = *value
	}
}

func assignString(target *string, value *string) {
	if value != nil {
		*target = strings.TrimSpace(*value)
	}
}

func assignBool(target *bool, value *bool) {
	if value != nil {
		*target = *value
	}
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

func runtimeToAdaptControllerConfig(cfg runtimeConfig) adapt.Config {
	return adapt.Config{
		ResourceID:        "",
		Mode:              "",
		TargetStart:       cfg.Controller.TargetStart,
		TargetMin:         cfg.Controller.TargetMin,
		TargetMax:         cfg.Controller.TargetMax,
		StepUp:            cfg.Controller.StepUp,
		StepDown:          cfg.Controller.StepDown,
		FallbackTarget:    cfg.Controller.FallbackTarget,
		GoalLow:           cfg.Controller.GoalLow,
		GoalHigh:          cfg.Controller.GoalHigh,
		Interval:          cfg.Controller.Interval,
		RelaxedInterval:   cfg.Controller.RelaxedInterval,
		RelaxedThreshold:  cfg.Controller.RelaxedThreshold,
		SuppressThreshold: cfg.Controller.SuppressThreshold,
		SuppressResume:    cfg.Controller.SuppressResume,
	}
}

func validateRuntimeConfig(cfg runtimeConfig) error {
	err := validatePoolSettings(cfg.Pool)
	if err != nil {
		return err
	}

	err := validateLoopIntervals(cfg.Controller, cfg.Estimator)
	if err != nil {
		return err
	}

	return validateControllerThresholds(cfg.Controller)
}

func validatePoolSettings(pool poolConfig) error {
	if pool.Workers <= 0 {
		return invalidConfigError("pool.workers (%d) must be greater than zero", pool.Workers)
	}

	return ensurePositiveDuration("pool.quantum", pool.Quantum)
}

func validateLoopIntervals(controller controllerConfig, estimator estimatorConfig) error {
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

	return nil
}

func validateControllerThresholds(controller controllerConfig) error {
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

func mergeRuntimeConfigFile(cfg *runtimeConfig, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("read config file %q: %w", path, err)
	}

	var fileCfg fileConfig

	err = yaml.Unmarshal(data, &fileCfg)
	if err != nil {
		return fmt.Errorf("decode config file %q: %w", path, err)
	}

	mergeControllerConfig(&cfg.Controller, fileCfg.Controller)
	mergeEstimatorConfig(&cfg.Estimator, fileCfg.Estimator)
	mergePoolConfig(&cfg.Pool, fileCfg.Pool)
	mergeHTTPConfig(&cfg.HTTP, fileCfg.HTTP)
	mergeOCIConfig(&cfg.OCI, fileCfg.OCI)

	return nil
}
