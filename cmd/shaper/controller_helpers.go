package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/est"
	"oci-cpu-shaper/pkg/imds"
	"oci-cpu-shaper/pkg/shape"
)

func handleControllerRunResult(logger *zap.Logger, runErr error) int {
	if runErr == nil {
		logger.Info("controller stopped", zap.String("reason", "completed"))

		return exitCodeSuccess
	}

	switch {
	case errors.Is(runErr, context.Canceled):
		logger.Info("controller stopped", zap.String("reason", context.Canceled.Error()))

		return exitCodeSuccess
	case errors.Is(runErr, context.DeadlineExceeded):
		logger.Info(
			"controller stopped",
			zap.String("reason", context.DeadlineExceeded.Error()),
		)

		return exitCodeSuccess
	default:
		logger.Error("controller execution failed", zap.Error(runErr))

		return exitCodeRuntimeError
	}
}

func exitCodeForConfigError(err error) int {
	if errors.Is(err, adapt.ErrInvalidConfig) {
		return exitCodeParseError
	}

	return exitCodeRuntimeError
}

//nolint:ireturn,funlen // helper returns controller interface for wiring and coordinates several setup steps
func buildAdaptiveController(
	ctx context.Context,
	mode string,
	cfg runtimeConfig,
	imdsClient imds.Client,
	recorder adapt.MetricsRecorder,
) (adapt.Controller, poolStarter, error) {
	offline := cfg.OCI.Offline

	instanceID, err := resolveInstanceID(ctx, cfg, offline, imdsClient)
	if err != nil {
		return nil, nil, err
	}

	compartmentID := strings.TrimSpace(cfg.OCI.CompartmentID)
	if compartmentID == "" && !offline {
		return nil, nil, errControllerCompartmentRequired
	}

	region := strings.TrimSpace(cfg.OCI.Region)
	if region == "" && !offline {
		return nil, nil, errControllerRegionRequired
	}

	metricsClient, err := createMetricsClient(ctx, cfg, offline, compartmentID, region)
	if err != nil {
		return nil, nil, err
	}

	pool, err := shape.NewPool(cfg.Pool.Workers, cfg.Pool.Quantum)
	if err != nil {
		return nil, nil, fmt.Errorf("build worker pool: %w", err)
	}

	pool.SetPauseThresholds(cfg.Pool.PauseThreshold, cfg.Pool.ResumeThreshold)

	sampler := est.NewSampler(nil, cfg.Estimator.Interval)

	controllerCfg := adapt.Config{
		ResourceID:        instanceID,
		Mode:              mode,
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

	controller, err := adapt.NewAdaptiveController(
		controllerCfg,
		metricsClient,
		sampler,
		pool,
		recorder,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("build adaptive controller: %w", err)
	}

	return controller, pool, nil
}
