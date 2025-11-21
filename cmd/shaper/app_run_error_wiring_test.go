package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/cgroup"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

var (
	errStubMetadataLookup        = errors.New("metadata lookup failed")
	errStubMetricsStartup        = errors.New("metrics startup failed")
	errUnexpectedControllerBuild = errors.New("unexpected controller construction")
)

func TestRunReturnsMetadataResolutionError(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	deps := defaultRunDeps()
	deps.detectCgroup = func() (*cgroup.CPU, error) {
		return &cgroup.CPU{
			Path:   "",
			Weight: cgroup.Weight{Path: "", Value: 0, Available: false, Err: nil},
			Max: cgroup.Max{
				Path:      "",
				Quota:     0,
				Period:    0,
				Unlimited: false,
				Available: false,
				Err:       nil,
			},
		}, nil
	}
	deps.loadConfig = func(string) (runtimeconfig.Config, error) {
		cfg := runtimeconfig.Default()
		cfg.OCI.Offline = false

		return cfg, nil
	}
	deps.newLogger = func(string) (*zap.Logger, error) {
		return logger, nil
	}
	deps.newIMDS = func() imds.Client {
		return &stubIMDSClient{compartmentErr: errStubMetadataLookup} //nolint:exhaustruct
	}
	deps.newController = func(
		context.Context,
		string,
		runtimeconfig.Config,
		imds.Client,
		adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error) {
		t.Fatal("unexpected controller construction")

		return nil, nil, errUnexpectedControllerBuild
	}

	exitCode := run(t.Context(), []string{"--mode", "enforce"}, deps, io.Discard)
	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected runtime error exit code, got %d", exitCode)
	}

	entries := observed.FilterMessage("failed to resolve oci metadata").All()
	if len(entries) == 0 {
		t.Fatalf("expected metadata resolution error log, got %+v", observed.All())
	}
}

//nolint:funlen // integration-style test exercises sequential wiring paths
func TestRunReturnsMetricsStartupError(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	deps := defaultRunDeps()
	deps.detectCgroup = func() (*cgroup.CPU, error) {
		return &cgroup.CPU{
			Path:   "",
			Weight: cgroup.Weight{Path: "", Value: 0, Available: false, Err: nil},
			Max: cgroup.Max{
				Path:      "",
				Quota:     0,
				Period:    0,
				Unlimited: false,
				Available: false,
				Err:       nil,
			},
		}, nil
	}
	deps.loadConfig = func(string) (runtimeconfig.Config, error) {
		cfg := runtimeconfig.Default()
		cfg.HTTP.Bind = "127.0.0.1:9000"
		cfg.OCI.Offline = true

		return cfg, nil
	}
	deps.newLogger = func(string) (*zap.Logger, error) {
		return logger, nil
	}
	deps.newIMDS = func() imds.Client { return &stubIMDSClient{} } //nolint:exhaustruct

	var ctrl stubController

	pool := new(stubPoolStarter)
	deps.newController = func(
		context.Context,
		string,
		runtimeconfig.Config,
		imds.Client,
		adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error) {
		ctrl.mode = modeDryRun

		return &ctrl, pool, nil
	}
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		return nil, errStubMetricsStartup
	}

	exitCode := run(t.Context(), []string{"--mode", "dry-run"}, deps, io.Discard)
	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected runtime error exit code, got %d", exitCode)
	}

	entries := observed.FilterMessage("failed to start metrics server").All()
	if len(entries) == 0 {
		t.Fatalf("expected metrics startup error log, got %+v", observed.All())
	}

	if ctrl.runCalled {
		t.Fatal("expected controller Run to be skipped when metrics startup fails")
	}
}

func TestRunHandlesControllerConstructionFailure(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	deps := defaultRunDeps()
	deps.detectCgroup = func() (*cgroup.CPU, error) {
		return &cgroup.CPU{
			Path:   "",
			Weight: cgroup.Weight{Path: "", Value: 0, Available: false, Err: nil},
			Max: cgroup.Max{
				Path:      "",
				Quota:     0,
				Period:    0,
				Unlimited: false,
				Available: false,
				Err:       nil,
			},
		}, nil
	}
	deps.loadConfig = func(string) (runtimeconfig.Config, error) {
		cfg := runtimeconfig.Default()
		cfg.OCI.Offline = true

		return cfg, nil
	}
	deps.newLogger = func(string) (*zap.Logger, error) {
		return logger, nil
	}
	deps.newIMDS = func() imds.Client { return &stubIMDSClient{} } //nolint:exhaustruct
	deps.newController = func(
		context.Context,
		string,
		runtimeconfig.Config,
		imds.Client,
		adapt.MetricsRecorder,
	) (adapt.Controller, poolStarter, error) {
		return nil, nil, adapt.ErrInvalidConfig
	}

	var stderr bytes.Buffer

	exitCode := run(t.Context(), []string{"--mode", "enforce"}, deps, &stderr)
	if exitCode != exitCodeParseError {
		t.Fatalf("expected parse error exit code, got %d", exitCode)
	}

	entries := observed.FilterMessage("failed to build controller").All()
	if len(entries) == 0 {
		t.Fatalf("expected controller build error log, got %+v", observed.All())
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected stderr to be empty when logger captures error, got %q", stderr.String())
	}
}
