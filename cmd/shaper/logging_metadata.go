package main

import (
	"context"
	"os"
	"strings"

	"go.uber.org/zap"
	"oci-cpu-shaper/internal/buildinfo"
	"oci-cpu-shaper/pkg/adapt"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

func logStartup(logger *zap.Logger, info buildinfo.Info, opts options) {
	fields := []zap.Field{
		zap.String("version", info.Version),
		zap.String("commit", info.GitCommit),
		zap.String("buildDate", info.BuildDate),
		zap.String("configPath", opts.configPath),
		zap.String("mode", opts.mode),
	}

	if opts.shutdownAfter > 0 {
		fields = append(fields, zap.Duration("shutdownAfter", opts.shutdownAfter))
	}

	logger.Info("starting oci-cpu-shaper", fields...)
}

func logRuntimeConfig(logger *zap.Logger, cfg runtimeconfig.Config) {
	if logger == nil {
		return
	}

	bind := strings.TrimSpace(cfg.HTTP.Bind)
	fields := []zap.Field{
		zap.Int("workerCount", cfg.Pool.Workers),
		zap.Duration("workerQuantum", cfg.Pool.Quantum),
		zap.Duration("estimatorInterval", cfg.Estimator.Interval),
		zap.Duration("controllerInterval", cfg.Controller.Interval),
		zap.Duration("controllerRelaxedInterval", cfg.Controller.RelaxedInterval),
		zap.Int("controllerRelaxedConfirmations", cfg.Controller.RelaxedConfirmations),
		zap.Float64("controllerTargetMin", cfg.Controller.TargetMin),
		zap.Float64("controllerTargetMax", cfg.Controller.TargetMax),
		zap.Float64("controllerGoalLow", cfg.Controller.GoalLow),
		zap.Float64("controllerGoalHigh", cfg.Controller.GoalHigh),
		zap.Float64("suppressThreshold", cfg.Controller.SuppressThreshold),
		zap.Float64("suppressResume", cfg.Controller.SuppressResume),
		zap.Float64("suppressRunnableThreshold", cfg.Controller.SuppressRunnableThreshold),
		zap.Float64("suppressRunnableResume", cfg.Controller.SuppressRunnableResume),
		zap.Float64("poolRunnableGuard", cfg.Pool.RunnableGuard),
		zap.Bool("offline", cfg.OCI.Offline),
		zap.Bool("httpEnabled", bind != ""),
	}

	if bind != "" {
		fields = append(fields, zap.String("httpBind", bind))
	}

	logger.Info("loaded runtime configuration", fields...)
}

func logMetadataResolution(
	logger *zap.Logger,
	mode string,
	metadata ociMetadata,
	offline bool,
) {
	if logger == nil {
		return
	}

	trimmedMode := strings.TrimSpace(mode)
	if trimmedMode == modeNoop {
		logger.Debug("metadata resolution skipped", zap.String("mode", trimmedMode))

		return
	}

	fields := []zap.Field{zap.Bool("offline", offline)}
	if trimmed := strings.TrimSpace(metadata.CompartmentID); trimmed != "" {
		fields = append(fields, zap.String("compartmentID", trimmed))
	}

	if trimmed := strings.TrimSpace(metadata.Region); trimmed != "" {
		fields = append(fields, zap.String("region", trimmed))
	}

	if offline {
		logger.Info("using offline metadata configuration", fields...)

		return
	}

	if metadata.CompartmentID == "" || metadata.Region == "" {
		logger.Warn("runtime metadata incomplete", fields...)

		return
	}

	logger.Info("resolved runtime metadata", fields...)
}

func logControllerInitialization(
	logger *zap.Logger,
	cfg runtimeconfig.Config,
	controller adapt.Controller,
	exporter *metricshttp.Exporter,
) {
	if logger == nil || controller == nil {
		return
	}

	fields := []zap.Field{
		zap.String("mode", controller.Mode()),
		zap.Bool("enforcingTargets", adapt.ModeEnforcesTargets(controller.Mode())),
		zap.String("controllerState", controller.State().String()),
		zap.Bool("offline", cfg.OCI.Offline),
		zap.Int("workerCount", cfg.Pool.Workers),
		zap.Duration("workerQuantum", cfg.Pool.Quantum),
		zap.Duration("estimatorInterval", cfg.Estimator.Interval),
		zap.Bool("metricsEnabled", exporter != nil),
	}

	if trimmed := strings.TrimSpace(cfg.OCI.CompartmentID); trimmed != "" {
		fields = append(fields, zap.String("compartmentID", trimmed))
	}

	if trimmed := strings.TrimSpace(cfg.OCI.Region); trimmed != "" {
		fields = append(fields, zap.String("region", trimmed))
	}

	logger.Info("controller initialized", fields...)
}

//nolint:ireturn // factory returns interface to support substitutable IMDS clients.
func defaultIMDSFactory() imds.Client {
	endpoint := strings.TrimSpace(os.Getenv(imdsEndpointEnv))

	var opts []imds.Option
	if endpoint != "" {
		opts = append(opts, imds.WithBaseURL(endpoint))
	}

	return imds.NewClient(nil, opts...)
}

func logIMDSMetadata(
	ctx context.Context,
	logger *zap.Logger,
	client imds.Client,
	controller adapt.Controller,
	overrideInstanceID string,
	overrideCompartmentID string,
	overrideRegion string,
	offline bool,
) {
	fields := []zap.Field{
		zap.String("controllerMode", controller.Mode()),
		zap.String("controllerState", controller.State().String()),
		zap.Bool("offline", offline),
	}

	trimmedOverride := strings.TrimSpace(overrideInstanceID)
	trimmedCompartment := strings.TrimSpace(overrideCompartmentID)
	trimmedRegion := strings.TrimSpace(overrideRegion)

	if offline {
		if trimmedOverride != "" {
			fields = append(fields, zap.String("instanceID", trimmedOverride))
		}

		logger.Info("initialized subsystems", fields...)

		return
	}

	fields = appendOnlineMetadata(
		ctx,
		logger,
		client,
		fields,
		trimmedOverride,
		trimmedCompartment,
		trimmedRegion,
	)

	logger.Info("initialized subsystems", fields...)
}

func queryTextMetadata(
	ctx context.Context,
	logger *zap.Logger,
	fetch func(context.Context) (string, error),
	warnMsg string,
) (string, error) {
	value, err := fetch(ctx)
	if err != nil {
		logger.Warn(warnMsg, zap.Error(err))

		return "", err
	}

	return value, nil
}

func queryShapeMetadata(
	ctx context.Context,
	logger *zap.Logger,
	fetch func(context.Context) (imds.ShapeConfig, error),
	warnMsg string,
) (imds.ShapeConfig, error) {
	value, err := fetch(ctx)
	if err != nil {
		logger.Warn(warnMsg, zap.Error(err))

		return imds.ShapeConfig{}, err
	}

	return value, nil
}

func appendStringField(fields []zap.Field, key, value string, err error) []zap.Field {
	if err != nil || strings.TrimSpace(value) == "" {
		return fields
	}

	return append(fields, zap.String(key, value))
}

func appendShapeFields(fields []zap.Field, shape imds.ShapeConfig, err error) []zap.Field {
	if err != nil {
		return fields
	}

	return append(fields,
		zap.Float64("shapeOCPUs", shape.OCPUs),
		zap.Float64("shapeMemoryGB", shape.MemoryInGBs),
	)
}

func resolveMetadataValue(
	ctx context.Context,
	logger *zap.Logger,
	override string,
	fetch func(context.Context) (string, error),
	warnMsg string,
) (string, error) {
	trimmed := strings.TrimSpace(override)
	if trimmed != "" {
		return trimmed, nil
	}

	return queryTextMetadata(ctx, logger, fetch, warnMsg)
}

func appendOnlineMetadata(
	ctx context.Context,
	logger *zap.Logger,
	client imds.Client,
	fields []zap.Field,
	overrideInstanceID string,
	overrideCompartmentID string,
	overrideRegion string,
) []zap.Field {
	region, regionErr := resolveMetadataValue(
		ctx,
		logger,
		overrideRegion,
		client.Region,
		"failed to query instance region",
	)

	canonicalRegion, canonicalRegionErr := resolveMetadataValue(
		ctx,
		logger,
		"",
		client.CanonicalRegion,
		"failed to query canonical region",
	)

	if canonicalRegionErr != nil || strings.TrimSpace(canonicalRegion) == "" {
		if trimmedOverride := strings.TrimSpace(overrideRegion); trimmedOverride != "" {
			canonicalRegion = trimmedOverride
			canonicalRegionErr = nil
		}
	}

	instanceID, instanceErr := resolveMetadataValue(
		ctx,
		logger,
		overrideInstanceID,
		client.InstanceID,
		"failed to query instance OCID",
	)

	compartmentID, compartmentErr := resolveMetadataValue(
		ctx,
		logger,
		overrideCompartmentID,
		client.CompartmentID,
		"failed to query compartment OCID",
	)

	shapeCfg, shapeErr := queryShapeMetadata(
		ctx,
		logger,
		client.ShapeConfig,
		"failed to query instance shape config",
	)

	fields = appendStringField(fields, "region", region, regionErr)
	fields = appendStringField(fields, "canonicalRegion", canonicalRegion, canonicalRegionErr)
	fields = appendStringField(fields, "instanceID", instanceID, instanceErr)
	fields = appendStringField(fields, "compartmentID", compartmentID, compartmentErr)

	return appendShapeFields(fields, shapeCfg, shapeErr)
}
