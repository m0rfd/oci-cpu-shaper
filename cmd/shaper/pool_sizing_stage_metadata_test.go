package main

import (
	"context"
	"io"
	"testing"

	"go.uber.org/zap"
	"oci-cpu-shaper/internal/buildinfo"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

func TestStageMetadataAutoSizesPool(
	t *testing.T,
) {
	t.Parallel()

	deps := defaultRunDeps()
	imdsClient := &stubIMDSClient{ //nolint:exhaustruct
		region:          "us-phoenix-1",
		canonicalRegion: "us-phoenix-1",
		instanceID:      "ocid1.instance.oc1..auto",
		compartmentID:   "ocid1.compartment.oc1..auto",
		shape:           stubShapeConfig(12, 0),
	}

	deps.newIMDS = func() imds.Client { return imdsClient }

	cfg := runtimeconfig.Default()
	cfg.Pool.AutoSizeFromShape = true

	boot := bootstrapResult{
		opts: options{
			configPath:    defaultConfigPath,
			logLevel:      defaultLogLevel,
			mode:          modeEnforce,
			shutdownAfter: 0,
			showVersion:   false,
		},
		cfg:    cfg,
		logger: zap.NewNop(),
		cancel: nil,
		info:   buildinfo.Info{Version: "", GitCommit: "", BuildDate: ""},
		stderr: io.Discard,
		deps:   deps,
	}

	result, exitCode, ready := stageMetadata(context.Background(), boot)
	if !ready {
		t.Fatalf("expected metadata stage to succeed, got exit code %d", exitCode)
	}

	if result.cfg.Pool.Workers != 12 {
		t.Fatalf("expected worker count to follow shape OCPUs, got %d", result.cfg.Pool.Workers)
	}

	if imdsClient.shapeCalls != 1 {
		t.Fatalf("expected ShapeConfig to be called once, got %d", imdsClient.shapeCalls)
	}
}

func TestStageMetadataAutoSizingFailure(
	t *testing.T,
) {
	t.Parallel()

	deps := defaultRunDeps()
	imdsClient := &stubIMDSClient{ //nolint:exhaustruct
		region:          "us-phoenix-1",
		canonicalRegion: "us-phoenix-1",
		instanceID:      "ocid1.instance.oc1..auto",
		compartmentID:   "ocid1.compartment.oc1..auto",
		shapeErr:        errShapeUnavailable,
	}

	deps.newIMDS = func() imds.Client { return imdsClient }

	cfg := runtimeconfig.Default()
	cfg.Pool.AutoSizeFromShape = true

	boot := bootstrapResult{
		opts: options{
			configPath:    defaultConfigPath,
			logLevel:      defaultLogLevel,
			mode:          modeEnforce,
			shutdownAfter: 0,
			showVersion:   false,
		},
		cfg:    cfg,
		logger: zap.NewNop(),
		cancel: nil,
		info:   buildinfo.Info{Version: "", GitCommit: "", BuildDate: ""},
		stderr: io.Discard,
		deps:   deps,
	}

	_, exitCode, ready := stageMetadata(context.Background(), boot)
	if ready {
		t.Fatal("expected metadata stage to fail when shape fetch fails")
	}

	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected runtime error exit code, got %d", exitCode)
	}
}
