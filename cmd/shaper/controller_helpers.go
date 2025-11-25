package main

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/est"
	"oci-cpu-shaper/pkg/imds"
	"oci-cpu-shaper/pkg/runtimeconfig"
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

//nolint:ireturn // helper returns controller interface for wiring and coordinates several setup steps
func buildAdaptiveController(
	ctx context.Context,
	mode string,
	cfg runtimeconfig.Config,
	imdsClient imds.Client,
	recorder adapt.MetricsRecorder,
) (adapt.Controller, poolStarter, error) {
	offline := cfg.OCI.Offline

	instanceID, err := resolveInstanceID(ctx, cfg, offline, imdsClient)
	if err != nil {
		return nil, nil, err
	}

	metadata, err := resolveCompartmentAndRegion(ctx, cfg, imdsClient)
	if err != nil {
		return nil, nil, err
	}

	if metadata.CompartmentID != "" {
		cfg.OCI.CompartmentID = metadata.CompartmentID
	}

	if metadata.Region != "" {
		cfg.OCI.Region = metadata.Region
	}

	compartmentID := metadata.CompartmentID
	region := metadata.Region

	metricsClient, err := createMetricsClient(ctx, cfg, offline, compartmentID, region)
	if err != nil {
		return nil, nil, err
	}

	pool, err := shape.NewPool(cfg.Pool.Workers, cfg.Pool.Quantum)
	if err != nil {
		return nil, nil, fmt.Errorf("build worker pool: %w", err)
	}

	pool.SetPauseThresholds(cfg.Pool.PauseThreshold, cfg.Pool.ResumeThreshold)
	pool.SetRunnableGuard(cfg.Pool.RunnableGuard)

	sampler := est.NewSampler(nil, cfg.Estimator.Interval)

	controllerCfg := cfg.ToAdaptConfig()
	controllerCfg.ResourceID = instanceID
	controllerCfg.Mode = mode

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
