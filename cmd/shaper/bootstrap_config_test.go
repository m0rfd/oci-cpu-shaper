package main

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"go.uber.org/zap"
	"oci-cpu-shaper/internal/buildinfo"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

var errStageConfigBoom = errors.New("boom")

func TestStageConfigReturnsConfigAndLogger(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	deps.loadConfig = func(string) (runtimeconfig.Config, error) { return runtimeconfig.Default(), nil }
	deps.newLogger = func(string) (*zap.Logger, error) { return zap.NewNop(), nil }
	deps.currentBuildInfo = func() buildinfo.Info {
		return buildinfo.Info{Version: "v1", GitCommit: "test", BuildDate: "now"}
	}

	opts := options{
		configPath:    "/tmp/config.yaml",
		logLevel:      "debug",
		mode:          modeDryRun,
		shutdownAfter: time.Second,
		showVersion:   false,
	}

	ctx, result, exitCode, ready := stageConfig(context.Background(), deps, opts, io.Discard)
	if !ready {
		t.Fatalf("expected stageConfig to proceed, got exit code %d", exitCode)
	}

	if exitCode != exitCodeSuccess {
		t.Fatalf("expected success exit code, got %d", exitCode)
	}

	if result.cfg.HTTP.Bind != runtimeconfig.Default().HTTP.Bind {
		t.Fatalf("expected default config, got bind=%q", result.cfg.HTTP.Bind)
	}

	if result.logger == nil {
		t.Fatal("expected logger to be configured")
	}

	if result.cancel == nil {
		t.Fatal("expected shutdown timer to be applied")
	}

	<-time.After(10 * time.Millisecond)

	if ctx.Err() != nil {
		t.Fatalf("expected derived context to remain usable before cancel, got %v", ctx.Err())
	}

	result.cancel()

	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("expected cancel to propagate, got %v", ctx.Err())
	}
}

func TestStageConfigHandlesConfigFailure(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	deps.loadConfig = func(string) (runtimeconfig.Config, error) {
		return runtimeconfig.Config{}, errStageConfigBoom
	}
	deps.currentBuildInfo = func() buildinfo.Info {
		return buildinfo.Info{Version: "", GitCommit: "", BuildDate: ""}
	}

	_, _, exitCode, ready := stageConfig(
		context.Background(),
		deps,
		options{
			configPath:    "missing",
			logLevel:      defaultLogLevel,
			mode:          modeDryRun,
			shutdownAfter: 0,
			showVersion:   false,
		},
		io.Discard,
	)
	if ready {
		t.Fatal("expected config load failure")
	}

	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected runtime error exit code, got %d", exitCode)
	}
}

func TestStageConfigHandlesLoggerFailure(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	deps.loadConfig = func(string) (runtimeconfig.Config, error) { return runtimeconfig.Default(), nil }
	deps.newLogger = func(string) (*zap.Logger, error) { return nil, errStageConfigBoom }
	deps.currentBuildInfo = func() buildinfo.Info {
		return buildinfo.Info{Version: "", GitCommit: "", BuildDate: ""}
	}

	_, _, exitCode, ready := stageConfig(
		context.Background(),
		deps,
		options{
			configPath:    defaultConfigPath,
			logLevel:      defaultLogLevel,
			mode:          modeDryRun,
			shutdownAfter: 0,
			showVersion:   false,
		},
		io.Discard,
	)
	if ready {
		t.Fatal("expected logger setup failure")
	}

	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected runtime error exit code, got %d", exitCode)
	}
}
