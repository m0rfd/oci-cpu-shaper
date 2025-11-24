package main

import (
	"context"
	"errors"
	"io"
	"testing"

	"go.uber.org/zap"
	"oci-cpu-shaper/internal/buildinfo"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

var errStageMetadataBoom = errors.New("boom")

func TestStageMetadataReturnsResolvedConfig(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	deps.newIMDS = func() imds.Client {
		return &stubIMDSClient{ //nolint:exhaustruct // test supplies only relevant metadata
			region:          "us-phoenix-1",
			canonicalRegion: "us-phoenix-1",
			instanceID:      "ocid1.instance.oc1..test",
			compartmentID:   "ocid1.compartment.oc1..test",
		}
	}

	boot := bootstrapResult{
		opts: options{
			configPath:    defaultConfigPath,
			logLevel:      defaultLogLevel,
			mode:          modeDryRun,
			shutdownAfter: 0,
			showVersion:   false,
		},
		cfg:    runtimeconfig.Default(),
		logger: zap.NewNop(),
		cancel: nil,
		info:   buildinfo.Info{Version: "", GitCommit: "", BuildDate: ""},
		stderr: io.Discard,
		deps:   deps,
	}

	result, exitCode, ready := stageMetadata(context.Background(), boot)
	if !ready {
		t.Fatalf("expected metadata resolution to succeed, got exit code %d", exitCode)
	}

	if result.cfg.OCI.Offline {
		t.Fatal("expected offline flag to remain false")
	}

	if result.imdsClient == nil {
		t.Fatal("expected IMDS client to be constructed")
	}
}

func TestStageMetadataReportsErrors(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()

	var failingIMDS stubIMDSClient

	failingIMDS.regionErr = errStageMetadataBoom
	deps.newIMDS = func() imds.Client { return &failingIMDS }

	boot := bootstrapResult{
		opts: options{
			configPath:    defaultConfigPath,
			logLevel:      defaultLogLevel,
			mode:          modeEnforce,
			shutdownAfter: 0,
			showVersion:   false,
		},
		cfg:    runtimeconfig.Default(),
		logger: zap.NewNop(),
		cancel: nil,
		info:   buildinfo.Info{Version: "", GitCommit: "", BuildDate: ""},
		stderr: io.Discard,
		deps:   deps,
	}

	_, exitCode, ready := stageMetadata(context.Background(), boot)
	if ready {
		t.Fatal("expected metadata resolution failure")
	}

	if exitCode != exitCodeRuntimeError {
		t.Fatalf("expected runtime error exit code, got %d", exitCode)
	}
}
