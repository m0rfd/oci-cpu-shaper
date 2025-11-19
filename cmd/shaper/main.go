// Package main wires the shaper CLI entrypoint.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"oci-cpu-shaper/internal/buildinfo"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/cgroup"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
	"oci-cpu-shaper/pkg/imds"
	"oci-cpu-shaper/pkg/oci"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

const (
	defaultConfigPath = "/etc/oci-cpu-shaper/config.yaml"
	defaultLogLevel   = "info"
	modeDryRun        = "dry-run"
	modeEnforce       = "enforce"
	modeNoop          = "noop"

	imdsEndpointEnv = "OCI_CPU_SHAPER_IMDS_ENDPOINT"

	offlineInstanceFallback = "offline-instance"

	exitCodeSuccess      = 0
	exitCodeRuntimeError = 1
	exitCodeParseError   = 2

	metricsReadHeaderTimeout = 5 * time.Second
	metricsShutdownTimeout   = 5 * time.Second
	cgroupLowWeightBaseline  = 128
)

func main() {
	code := newApp(defaultRunDeps()).Run(context.Background(), os.Args[1:], os.Stderr)
	if code != 0 {
		exitProcess(code)
	}
}

var exitProcess = os.Exit //nolint:gochecknoglobals // replaceable for tests

type runDeps struct {
	newLogger     func(level string) (*zap.Logger, error)
	newIMDS       func() imds.Client
	newController func(
		ctx context.Context,
		mode string,
		cfg runtimeconfig.Config,
		imdsClient imds.Client,
		recorder adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error)
	currentBuildInfo   func() buildinfo.Info
	loadConfig         func(path string) (runtimeconfig.Config, error)
	newMetricsExporter func() *metricshttp.Exporter
	startMetricsServer func(
		ctx context.Context,
		logger *zap.Logger,
		addr string,
		handler http.Handler,
	) (metricsShutdownFunc, error)
	versionWriter io.Writer
	detectCgroup  func() (*cgroup.CPU, error)
}

type poolStarter interface {
	Start(ctx context.Context)
	Workers() int
	Quantum() time.Duration
	SetWorkerStartErrorHandler(handler func(err error))
}

type metricsClientFactory func(compartmentID, region string) (oci.MetricsClient, error)

type metricsClientFactoryKey struct{}

func withMetricsClientFactory(ctx context.Context, factory metricsClientFactory) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	if factory == nil {
		return ctx
	}

	return context.WithValue(ctx, metricsClientFactoryKey{}, factory)
}

func metricsClientFactoryFromContext(ctx context.Context) metricsClientFactory {
	if ctx != nil {
		if factory, ok := ctx.Value(metricsClientFactoryKey{}).(metricsClientFactory); ok &&
			factory != nil {
			return factory
		}
	}

	return buildInstancePrincipalMetricsClient
}

var (
	errControllerIMDSRequired        = errors.New("controller factory: imds client is required")
	errControllerCompartmentRequired = errors.New(
		"controller factory: OCI compartment ID is required",
	)
	errControllerRegionRequired = errors.New("controller factory: OCI region is required")
	errMetricsDelegateNil       = errors.New("metrics client: nil delegate")
	errMetricsContextRequired   = errors.New("metrics server: context is required")
	errMetricsServerDisabled    = errors.New("metrics server: disabled")
)

func writeError(dst io.Writer, err error, code int) int {
	if err == nil {
		return code
	}

	_, ferr := fmt.Fprintf(dst, "%v\n", err)
	if ferr != nil {
		return code
	}

	return code
}

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

func loadRuntimeConfigOrExit(
	deps runDeps,
	path string,
	stderr io.Writer,
) (runtimeconfig.Config, int, bool) {
	cfg, loadErr := deps.loadConfig(path)
	if loadErr != nil {
		code := exitCodeForConfigError(loadErr)

		exitCode := writeError(
			stderr,
			fmt.Errorf("failed to load configuration: %w", loadErr),
			code,
		)

		var empty runtimeconfig.Config

		return empty, exitCode, false
	}

	return cfg, exitCodeSuccess, true
}

func buildLoggerOrExit(
	deps runDeps,
	level string,
	stderr io.Writer,
) (*zap.Logger, int, bool) {
	logger, loggerErr := deps.newLogger(level)
	if loggerErr != nil {
		exitCode := writeError(
			stderr,
			fmt.Errorf("failed to configure logger: %w", loggerErr),
			exitCodeRuntimeError,
		)

		return nil, exitCode, false
	}

	return logger, exitCodeSuccess, true
}

//nolint:ireturn // factory intentionally returns controller interface for wiring flexibility.
func defaultControllerFactory(
	ctx context.Context,
	mode string,
	cfg runtimeconfig.Config,
	imdsClient imds.Client,
	recorder adapt.MetricsRecorder,
) (adapt.Controller, poolStarter, error) {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" {
		trimmed = modeDryRun
	}

	if trimmed == modeNoop {
		if recorder != nil {
			recorder.SetMode(trimmed)
			recorder.SetState(adapt.StateNormal.String())
			recorder.SetTarget(0)
		}

		return adapt.NewNoopController(trimmed), nil, nil
	}

	if imdsClient == nil {
		return nil, nil, errControllerIMDSRequired
	}

	return buildAdaptiveController(ctx, trimmed, cfg, imdsClient, recorder)
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

	if cfg.OCI.Offline {
		return ociMetadata{
			CompartmentID: compartmentOverride,
			Region:        regionOverride,
		}, nil
	}

	if imdsClient == nil {
		return ociMetadata{}, errControllerIMDSRequired
	}

	compartmentID, compartmentErr := imdsClient.CompartmentID(ctx)
	region, regionErr := imdsClient.Region(ctx)

	var metadata ociMetadata

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

	value, err = preferMetadataValue(
		region,
		regionErr,
		regionOverride,
		errControllerRegionRequired,
		"lookup instance region",
	)
	if err != nil {
		return ociMetadata{}, err
	}

	metadata.Region = value

	return metadata, nil
}

func preferMetadataValue(
	fetched string,
	fetchErr error,
	override string,
	missingErr error,
	errPrefix string,
) (string, error) {
	trimmedFetched := strings.TrimSpace(fetched)
	if trimmedFetched != "" {
		return trimmedFetched, nil
	}

	if override != "" {
		return override, nil
	}

	if fetchErr != nil {
		return "", fmt.Errorf("%s: %w", errPrefix, fetchErr)
	}

	return "", missingErr
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

func applyShutdownTimer(
	ctx context.Context,
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, nil
	}

	newCtx, cancel := context.WithTimeout(ctx, timeout)

	return newCtx, cancel
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

//nolint:ireturn // helper returns MetricsClient interface for dependency substitution.
func createMetricsClient(
	ctx context.Context,
	cfg runtimeconfig.Config,
	offline bool,
	compartmentID string,
	region string,
) (oci.MetricsClient, error) {
	if offline {
		return oci.NewStaticMetricsClient(cfg.Controller.TargetStart), nil
	}

	factory := metricsClientFactoryFromContext(ctx)

	metricsClient, err := factory(compartmentID, region)
	if err != nil {
		return nil, fmt.Errorf("build monitoring client: %w", err)
	}

	return metricsClient, nil
}

func startMetricsServer( //nolint:cyclop,funlen // listener lifecycle requires several guard branches
	ctx context.Context,
	logger *zap.Logger,
	addr string,
	handler http.Handler,
) (metricsShutdownFunc, error) {
	trimmed := strings.TrimSpace(addr)

	if logger == nil {
		logger = zap.NewNop()
	}

	if trimmed == "" {
		logger.Info("metrics server disabled", zap.String("reason", "http bind address empty"))

		return nil, errMetricsServerDisabled
	}

	if handler == nil {
		logger.Warn("metrics server disabled", zap.String("reason", "handler missing"))

		return nil, errMetricsServerDisabled
	}

	if ctx == nil {
		return nil, errMetricsContextRequired
	}

	baseCtx := context.WithoutCancel(ctx)

	var listenCfg net.ListenConfig

	listener, err := listenCfg.Listen(ctx, "tcp", trimmed)
	if err != nil {
		logger.Error(
			"metrics server listen failed",
			zap.String("bind", trimmed),
			zap.Error(err),
		)

		return nil, fmt.Errorf("listen metrics endpoint %q: %w", trimmed, err)
	}

	server := &http.Server{ //nolint:exhaustruct // only security-critical timeout configured here
		ReadHeaderTimeout: metricsReadHeaderTimeout,
	}
	server.Addr = trimmed
	server.Handler = handler

	logger.Info("metrics server listening", zap.String("bind", trimmed))

	serveDone := make(chan struct{})

	go func() {
		defer close(serveDone)

		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("metrics server serve", zap.String("bind", trimmed), zap.Error(err))

			return
		}

		logger.Info("metrics server stopped", zap.String("bind", trimmed))
	}()

	shutdown := func(shutdownCtx context.Context) {
		if shutdownCtx == nil {
			shutdownCtx = baseCtx
		}

		logger.Info("stopping metrics server", zap.String("bind", trimmed))

		err := server.Shutdown(shutdownCtx)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Warn("metrics server shutdown", zap.String("bind", trimmed), zap.Error(err))
		}
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(baseCtx, metricsShutdownTimeout)
		defer cancel()

		shutdown(shutdownCtx)
	}()

	return func(shutdownCtx context.Context) {
		shutdown(shutdownCtx)

		<-serveDone
	}, nil
}

type p95CPUQuerier interface {
	QueryP95CPU(ctx context.Context, resourceID string, last7d bool) (float32, error)
}

type instancePrincipalMetricsClient struct {
	client p95CPUQuerier
}

func (m *instancePrincipalMetricsClient) QueryP95CPU(
	ctx context.Context,
	resourceID string,
) (float64, error) {
	if m == nil || m.client == nil {
		return 0, errMetricsDelegateNil
	}

	value, err := m.client.QueryP95CPU(ctx, resourceID, true)
	if err != nil {
		return 0, fmt.Errorf("query p95 cpu: %w", err)
	}

	return float64(value), nil
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

func detectAndReportCgroup(
	deps runDeps,
	logger *zap.Logger,
	exporter *metricshttp.Exporter,
) *cgroup.CPU {
	info, err := detectCgroupInfo(deps)
	if err != nil {
		if logger != nil {
			logger.Warn("failed to inspect cgroup cpu settings", zap.Error(err))
		}

		recordCgroupMetrics(exporter, nil)

		return nil
	}

	recordCgroupMetrics(exporter, info)
	logCgroupInfo(logger, info)

	return info
}

func detectCgroupInfo(deps runDeps) (*cgroup.CPU, error) {
	if deps.detectCgroup != nil {
		return deps.detectCgroup()
	}

	var reader cgroup.Reader

	info, err := reader.Detect()
	if err != nil {
		return nil, fmt.Errorf("detect cgroup: %w", err)
	}

	return info, nil
}

func recordCgroupMetrics(exporter *metricshttp.Exporter, info *cgroup.CPU) {
	if exporter == nil {
		return
	}

	weight := uint64(0)
	if info != nil && info.Weight.Err == nil && info.Weight.Available {
		weight = info.Weight.Value
	}

	exporter.SetCgroupCPUWeight(weight)

	var (
		quota  uint64
		period uint64
	)

	unlimited := false

	if info != nil && info.Max.Err == nil && info.Max.Available {
		period = info.Max.Period

		unlimited = info.Max.Unlimited
		if !info.Max.Unlimited {
			quota = info.Max.Quota
		}
	}

	exporter.SetCgroupCPUMax(quota, period, unlimited)
}

func logCgroupInfo(logger *zap.Logger, info *cgroup.CPU) {
	if logger == nil || info == nil {
		return
	}

	fields := []zap.Field{zap.String("path", strings.TrimSpace(info.Path))}
	fields = append(fields, cgroupWeightFields(logger, info.Weight)...)
	fields = append(fields, cgroupMaxFields(info.Max)...)

	logger.Info("detected cgroup cpu settings", fields...)
}

func cgroupWeightFields(logger *zap.Logger, weight cgroup.Weight) []zap.Field {
	switch {
	case weight.Err != nil:
		return []zap.Field{zap.String("cpuWeightError", weight.Err.Error())}
	case weight.Available:
		field := zap.Uint64("cpuWeight", weight.Value)
		if logger != nil && weight.Value > cgroupLowWeightBaseline {
			logger.Warn(
				"cpu.weight exceeds recommended low-weight baseline",
				zap.Uint64("weight", weight.Value),
				zap.Uint64("baseline", cgroupLowWeightBaseline),
			)
		}

		return []zap.Field{field}
	default:
		return []zap.Field{zap.String("cpuWeightStatus", "unavailable")}
	}
}

func cgroupMaxFields(cpuMax cgroup.Max) []zap.Field {
	switch {
	case cpuMax.Err != nil:
		return []zap.Field{zap.String("cpuMaxError", cpuMax.Err.Error())}
	case cpuMax.Available:
		fields := []zap.Field{
			zap.Bool("cpuMaxUnlimited", cpuMax.Unlimited),
			zap.Uint64("cpuMaxPeriod", cpuMax.Period),
		}
		if !cpuMax.Unlimited {
			fields = append(fields, zap.Uint64("cpuMaxQuota", cpuMax.Quota))
		}

		return fields
	default:
		return []zap.Field{zap.String("cpuMaxStatus", "unavailable")}
	}
}
