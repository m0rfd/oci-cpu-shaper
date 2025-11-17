// Package main wires the shaper CLI entrypoint.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
	"oci-cpu-shaper/internal/buildinfo"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/est"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
	statushttp "oci-cpu-shaper/pkg/http/status"
	"oci-cpu-shaper/pkg/imds"
	"oci-cpu-shaper/pkg/oci"
	"oci-cpu-shaper/pkg/shape"
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
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-sigCh:
			cancel()
		}
	}()

	code := run(ctx, os.Args[1:], defaultRunDeps(), os.Stderr)
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
		cfg runtimeConfig,
		imdsClient imds.Client,
		recorder adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error)
	currentBuildInfo   func() buildinfo.Info
	loadConfig         func(path string) (runtimeConfig, error)
	newMetricsExporter func() *metricshttp.Exporter
	startMetricsServer func(
		ctx context.Context,
		logger *zap.Logger,
		addr string,
		handler http.Handler,
	) (metricsShutdownFunc, error)
	versionWriter io.Writer
}

type poolStarter interface {
	Start(ctx context.Context)
	Workers() int
	Quantum() time.Duration
	SetWorkerStartErrorHandler(handler func(err error))
}

type metricsClientFactory func(compartmentID, region string) (oci.MetricsClient, error)

type metricsClientFactoryKey struct{}

type metricsShutdownFunc func(context.Context)

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

func buildMetricsExporter(deps runDeps) *metricshttp.Exporter {
	if deps.newMetricsExporter != nil {
		exporter := deps.newMetricsExporter()
		if exporter != nil {
			return exporter
		}
	}

	return metricshttp.NewExporter()
}

func configureMetrics(
	logger *zap.Logger,
	exporter *metricshttp.Exporter,
	pool poolStarter,
	controller adapt.Controller,
) http.Handler {
	if logger == nil {
		logger = zap.NewNop()
	}

	if exporter == nil {
		logger.Warn("metrics exporter disabled", zap.String("reason", "no exporter configured"))

		return nil
	}

	if pool != nil {
		workers := pool.Workers()
		exporter.SetWorkerCount(workers)

		quantum := pool.Quantum()
		exporter.SetDutyCycle(quantum)

		logger.Debug(
			"registered worker pool metrics",
			zap.Int("workerCount", workers),
			zap.Duration("dutyCycle", quantum),
		)
	} else {
		logger.Debug("worker pool metrics unavailable", zap.String("reason", "pool not configured"))
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", exporter)

	if controller != nil {
		mux.Handle("/healthz", statushttp.NewHandler(controller))
	}

	return mux
}

func startMetricsEndpoint(
	ctx context.Context,
	deps runDeps,
	logger *zap.Logger,
	bindAddr string,
	handler http.Handler,
) (metricsShutdownFunc, context.CancelFunc, error) {
	if handler == nil {
		return nil, nil, nil
	}

	if deps.startMetricsServer == nil {
		logger.Warn("metrics server disabled", zap.String("reason", "start function missing"))

		return nil, nil, nil
	}

	trimmed := strings.TrimSpace(bindAddr)
	if trimmed == "" {
		logger.Info("metrics server disabled", zap.String("reason", "http bind address empty"))

		return nil, nil, nil
	}

	if ctx == nil {
		return nil, nil, errMetricsContextRequired
	}

	logger.Info("starting metrics server", zap.String("bind", trimmed))

	metricsCtx, cancel := context.WithCancel(ctx)

	shutdown, err := deps.startMetricsServer(metricsCtx, logger, trimmed, handler)
	if err != nil {
		cancel()

		return nil, nil, err
	}

	return shutdown, cancel, nil
}

// run orchestrates CLI initialization before handing execution to the controller.
//
//nolint:funlen,cyclop // CLI wiring composes setup steps before controller execution
func run(
	ctx context.Context,
	args []string,
	deps runDeps,
	stderr io.Writer,
) int {
	opts, err := parseArgs(args)
	if err != nil {
		return writeError(stderr, err, exitCodeParseError)
	}

	if opts.showVersion {
		info := deps.currentBuildInfo()

		writer := deps.versionWriter
		if writer == nil {
			writer = os.Stdout
		}

		_, _ = fmt.Fprintf(writer, "%+v\n", info)

		return exitCodeSuccess
	}

	cfg, exitCode, configLoaded := loadRuntimeConfigOrExit(deps, opts.configPath, stderr)
	if !configLoaded {
		return exitCode
	}

	logger, exitCode, loggerReady := buildLoggerOrExit(deps, opts.logLevel, stderr)
	if !loggerReady {
		return exitCode
	}

	defer func() {
		_ = logger.Sync()
	}()

	ctx, cancel := applyShutdownTimer(ctx, opts.shutdownAfter)
	if cancel != nil {
		defer cancel()
	}

	info := deps.currentBuildInfo()
	logStartup(logger, info, opts)
	logRuntimeConfig(logger, cfg)

	imdsClient := deps.newIMDS()

	metricsExporter := buildMetricsExporter(deps)
	metricsRecorder := newRecorderLogger(logger, metricsExporter)

	cfg, metadata, metadataErr := prepareRunMetadata(ctx, cfg, imdsClient, opts.mode)
	if metadataErr != nil {
		logger.Error("failed to resolve oci metadata", zap.Error(metadataErr))

		return exitCodeRuntimeError
	}

	logMetadataResolution(logger, opts.mode, metadata, cfg.OCI.Offline)

	controller, pool, buildErr := deps.newController(
		ctx,
		opts.mode,
		cfg,
		imdsClient,
		metricsRecorder,
	)
	if buildErr != nil {
		code := exitCodeForConfigError(buildErr)

		logger.Error("failed to build controller", zap.Error(buildErr))

		return code
	}

	logControllerInitialization(logger, cfg, controller, metricsExporter)

	metricsHandler := configureMetrics(logger, metricsExporter, pool, controller)

	metricsShutdown, metricsCancel, metricsErr := startMetricsEndpoint(
		ctx,
		deps,
		logger,
		cfg.HTTP.Bind,
		metricsHandler,
	)
	if metricsErr != nil {
		logger.Error("failed to start metrics server", zap.Error(metricsErr))

		return exitCodeRuntimeError
	}

	if metricsShutdown != nil {
		defer func() {
			if metricsCancel != nil {
				metricsCancel()
			}

			shutdownCtx, cancelShutdown := context.WithTimeout(
				context.WithoutCancel(ctx),
				metricsShutdownTimeout,
			)
			defer cancelShutdown()

			metricsShutdown(shutdownCtx)
		}()
	}

	if pool != nil {
		pool.SetWorkerStartErrorHandler(func(err error) {
			if err == nil {
				return
			}

			logger.Warn("worker failed to enter sched_idle", zap.Error(err))
		})

		logger.Info(
			"starting worker pool",
			zap.Int("workers", pool.Workers()),
			zap.Duration("quantum", pool.Quantum()),
		)

		pool.Start(ctx)
	}

	logIMDSMetadata(
		ctx,
		logger,
		imdsClient,
		controller,
		cfg.OCI.InstanceID,
		cfg.OCI.CompartmentID,
		cfg.OCI.Region,
		cfg.OCI.Offline,
	)

	logger.Info(
		"starting controller run",
		zap.String("mode", controller.Mode()),
		zap.String("controllerState", controller.State().String()),
	)

	return handleControllerRunResult(logger, controller.Run(ctx))
}

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

type options struct {
	configPath    string
	logLevel      string
	mode          string
	shutdownAfter time.Duration
	showVersion   bool
}

func parseArgs(args []string) (options, error) {
	var opts options

	flagSet := flag.NewFlagSet("shaper", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	flagSet.BoolVar(
		&opts.showVersion,
		"version",
		false,
		"Print build information and exit",
	)
	flagSet.StringVar(
		&opts.configPath,
		"config",
		defaultConfigPath,
		"Path to the shaper configuration file",
	)
	flagSet.StringVar(
		&opts.logLevel,
		"log-level",
		defaultLogLevel,
		"Structured log level (debug, info, warn, error)",
	)
	flagSet.StringVar(
		&opts.mode,
		"mode",
		modeDryRun,
		"Controller mode to use (dry-run, enforce, noop)",
	)
	flagSet.DurationVar(
		&opts.shutdownAfter,
		"shutdown-after",
		0,
		"Gracefully stop the controller after the provided duration (0 disables the timer)",
	)

	err := flagSet.Parse(args)
	if err != nil {
		return options{}, fmt.Errorf("parse CLI arguments: %w", err)
	}

	if !opts.showVersion {
		if slices.Contains(flagSet.Args(), "version") {
			opts.showVersion = true
		}
	}

	if opts.showVersion {
		return opts, nil
	}

	normErr := normalizeOptions(&opts)
	if normErr != nil {
		return options{}, normErr
	}

	return opts, nil
}

func normalizeOptions(opts *options) error {
	if opts == nil {
		return nil
	}

	opts.mode = strings.ToLower(strings.TrimSpace(opts.mode))
	if opts.mode == "" {
		opts.mode = modeDryRun
	}

	if !isValidMode(opts.mode) {
		return fmt.Errorf(
			"%w: %q (supported: %s, %s, %s)",
			errUnsupportedMode,
			opts.mode,
			modeDryRun,
			modeEnforce,
			modeNoop,
		)
	}

	opts.logLevel = strings.TrimSpace(opts.logLevel)
	if opts.logLevel == "" {
		opts.logLevel = defaultLogLevel
	}

	opts.configPath = strings.TrimSpace(opts.configPath)
	if opts.configPath == "" {
		opts.configPath = defaultConfigPath
	}

	if opts.shutdownAfter < 0 {
		return fmt.Errorf("%w: %v", errInvalidShutdownAfter, opts.shutdownAfter)
	}

	return nil
}

func loadRuntimeConfigOrExit(
	deps runDeps,
	path string,
	stderr io.Writer,
) (runtimeConfig, int, bool) {
	cfg, loadErr := deps.loadConfig(path)
	if loadErr != nil {
		code := exitCodeForConfigError(loadErr)

		exitCode := writeError(
			stderr,
			fmt.Errorf("failed to load configuration: %w", loadErr),
			code,
		)

		var empty runtimeConfig

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

var (
	errInvalidLogLevel      = errors.New("invalid log level")
	errUnsupportedMode      = errors.New("unsupported mode provided")
	errInvalidShutdownAfter = errors.New("invalid shutdown-after duration (must be >=0)")
)

//nolint:ireturn // factory intentionally returns controller interface for wiring flexibility.
func defaultControllerFactory(
	ctx context.Context,
	mode string,
	cfg runtimeConfig,
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

func resolveInstanceID(
	ctx context.Context,
	cfg runtimeConfig,
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
	cfg runtimeConfig,
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
	cfg runtimeConfig,
	imdsClient imds.Client,
	mode string,
) (runtimeConfig, ociMetadata, error) {
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

func logRuntimeConfig(logger *zap.Logger, cfg runtimeConfig) {
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
	cfg runtimeConfig,
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
	cfg runtimeConfig,
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
		overrideRegion,
		client.CanonicalRegion,
		"failed to query canonical region",
	)

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

func isValidMode(mode string) bool {
	switch mode {
	case modeDryRun, modeEnforce, modeNoop:
		return true
	default:
		return false
	}
}
