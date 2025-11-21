package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"oci-cpu-shaper/internal/buildinfo"
	"oci-cpu-shaper/pkg/adapt"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

var (
	errControllerIMDSRequired        = errors.New("controller factory: imds client is required")
	errControllerCompartmentRequired = errors.New(
		"controller factory: OCI compartment ID is required",
	)
	errControllerRegionRequired = errors.New("controller factory: OCI region is required")
)

func newLogger(level string) (*zap.Logger, error) {
	if level == "" {
		level = defaultLogLevel
	}

	cfg := zap.NewProductionConfig()

	err := cfg.Level.UnmarshalText([]byte(level))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidLogLevel, err)
	}

	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.MessageKey = "message"
	cfg.EncoderConfig.LevelKey = "level"
	cfg.EncoderConfig.CallerKey = "caller"

	logger, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("build zap logger: %w", err)
	}

	return logger, nil
}

func resolveInstanceID(
	ctx context.Context,
	cfg runtimeconfig.Config,
	offline bool,
	imdsClient imds.Client,
) (string, error) {
	instanceID := strings.TrimSpace(cfg.OCI.InstanceID)
	if instanceID != "" {
		return instanceID, nil
	}

	if offline {
		return offlineInstanceFallback, nil
	}

	fetchedID, err := imdsClient.InstanceID(ctx)
	if err != nil {
		return "", fmt.Errorf("lookup instance ocid: %w", err)
	}

	return strings.TrimSpace(fetchedID), nil
}

type ociMetadata struct {
	CompartmentID string
	Region        string
}

func resolveCompartmentAndRegion(
	ctx context.Context,
	cfg runtimeconfig.Config,
	imdsClient imds.Client,
) (ociMetadata, error) {
	compartmentOverride := strings.TrimSpace(cfg.OCI.CompartmentID)
	regionOverride := strings.TrimSpace(cfg.OCI.Region)

	metadata := ociMetadata{
		CompartmentID: compartmentOverride,
		Region:        regionOverride,
	}

	if cfg.OCI.Offline {
		return metadata, nil
	}

	if imdsClient == nil {
		return ociMetadata{}, errControllerIMDSRequired
	}

	if metadata.CompartmentID != "" && metadata.Region != "" {
		return metadata, nil
	}

	if metadata.CompartmentID == "" {
		compartmentID, compartmentErr := imdsClient.CompartmentID(ctx)

		value, err := preferMetadataValue(
			compartmentID,
			compartmentErr,
			compartmentOverride,
			errControllerCompartmentRequired,
			"lookup compartment ocid",
		)
		if err != nil {
			return ociMetadata{}, err
		}

		metadata.CompartmentID = value
	}

	if metadata.Region == "" {
		canonicalRegion, canonicalRegionErr := imdsClient.CanonicalRegion(ctx)
		region, regionErr := imdsClient.Region(ctx)

		value, err := preferCanonicalRegionValue(
			canonicalRegion,
			canonicalRegionErr,
			region,
			regionErr,
			regionOverride,
		)
		if err != nil {
			return ociMetadata{}, err
		}

		metadata.Region = value
	}

	return metadata, nil
}

func preferMetadataValue(
	fetched string,
	fetchErr error,
	override string,
	missingErr error,
	errPrefix string,
) (string, error) {
	trimmedOverride := strings.TrimSpace(override)
	if trimmedOverride != "" {
		return trimmedOverride, nil
	}

	trimmedFetched := strings.TrimSpace(fetched)
	if trimmedFetched != "" {
		return trimmedFetched, nil
	}

	if fetchErr != nil {
		return "", fmt.Errorf("%s: %w", errPrefix, fetchErr)
	}

	return "", missingErr
}

func preferCanonicalRegionValue(
	canonical string,
	canonicalErr error,
	legacy string,
	legacyErr error,
	override string,
) (string, error) {
	trimmedOverride := strings.TrimSpace(override)
	if trimmedOverride != "" {
		return trimmedOverride, nil
	}

	trimmedCanonical := strings.TrimSpace(canonical)
	if trimmedCanonical != "" && canonicalErr == nil {
		return trimmedCanonical, nil
	}

	trimmedLegacy := strings.TrimSpace(legacy)
	if trimmedLegacy != "" && legacyErr == nil {
		return trimmedLegacy, nil
	}

	if trimmedLegacy != "" {
		return trimmedLegacy, nil
	}

	if legacyErr != nil {
		return "", fmt.Errorf("lookup instance region: %w", legacyErr)
	}

	if canonicalErr != nil {
		return "", fmt.Errorf("lookup canonical region: %w", canonicalErr)
	}

	return "", errControllerRegionRequired
}

func prepareRunMetadata(
	ctx context.Context,
	cfg runtimeconfig.Config,
	imdsClient imds.Client,
	mode string,
) (runtimeconfig.Config, ociMetadata, error) {
	trimmedMode := strings.TrimSpace(mode)
	if trimmedMode == modeNoop {
		var empty ociMetadata

		return cfg, empty, nil
	}

	metadata, err := resolveCompartmentAndRegion(ctx, cfg, imdsClient)
	if err != nil {
		return cfg, ociMetadata{}, err
	}

	if metadata.CompartmentID != "" {
		cfg.OCI.CompartmentID = metadata.CompartmentID
	}

	if metadata.Region != "" {
		cfg.OCI.Region = metadata.Region
	}

	return cfg, metadata, nil
}

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
		zap.Float64("controllerTargetMin", cfg.Controller.TargetMin),
		zap.Float64("controllerTargetMax", cfg.Controller.TargetMax),
		zap.Float64("controllerGoalLow", cfg.Controller.GoalLow),
		zap.Float64("controllerGoalHigh", cfg.Controller.GoalHigh),
		zap.Float64("suppressThreshold", cfg.Controller.SuppressThreshold),
		zap.Float64("suppressResume", cfg.Controller.SuppressResume),
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
