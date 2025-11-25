package runtimeconfig

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func mergeRuntimeConfigFile(cfg *Config, path string) error {
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

func mergeControllerConfig(dst *ControllerConfig, src controllerFileConfig) {
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
	assignInt(&dst.RelaxedConfirmations, src.RelaxedConfirmations)
	assignFloat(&dst.SuppressThreshold, src.SuppressThreshold)
	assignFloat(&dst.SuppressResume, src.SuppressResume)
	assignFloat(&dst.SuppressRunnableThreshold, src.SuppressRunnableThreshold)
	assignFloat(&dst.SuppressRunnableResume, src.SuppressRunnableResume)
}

func mergeEstimatorConfig(dst *EstimatorConfig, src estimatorFileConfig) {
	assignDuration(&dst.Interval, src.Interval)
}

func mergePoolConfig(dst *PoolConfig, src poolFileConfig) {
	assignInt(&dst.Workers, src.Workers)
	assignDuration(&dst.Quantum, src.Quantum)
	assignFloat(&dst.PauseThreshold, src.PauseThreshold)
	assignFloat(&dst.ResumeThreshold, src.ResumeThreshold)
}

func mergeHTTPConfig(dst *HTTPConfig, src httpFileConfig) {
	assignString(&dst.Bind, src.Bind)
}

func mergeOCIConfig(dst *OCIConfig, src ociFileConfig) {
	assignString(&dst.CompartmentID, src.CompartmentID)
	assignString(&dst.Region, src.Region)
	assignString(&dst.InstanceID, src.InstanceID)
	assignBool(&dst.Offline, src.Offline)
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
