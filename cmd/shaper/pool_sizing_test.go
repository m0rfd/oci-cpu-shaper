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

var (
	errShapeLookupFailed = errors.New("shape lookup failed")
	errShapeUnavailable  = errors.New("shape unavailable")
)

func TestApplyPoolSizingFromShapeDisabled(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.Pool.Workers = 3

	imds := &stubIMDSClient{shape: stubShapeConfig(4, 0)} //nolint:exhaustruct

	updated, result, err := applyPoolSizingFromShape(context.Background(), cfg, modeEnforce, imds)
	if err != nil {
		t.Fatalf("applyPoolSizingFromShape returned error: %v", err)
	}

	if result.applied {
		t.Fatal("expected sizing to be disabled")
	}

	if updated.Pool.Workers != cfg.Pool.Workers {
		t.Fatalf(
			"expected worker count to remain %d, got %d",
			cfg.Pool.Workers,
			updated.Pool.Workers,
		)
	}

	if imds.shapeCalls != 0 {
		t.Fatalf("expected shape config to be skipped, got %d calls", imds.shapeCalls)
	}
}

func TestApplyPoolSizingFromShapeUsesOCPUs(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.Pool.AutoSizeFromShape = true

	imds := &stubIMDSClient{shape: stubShapeConfig(6.1, 0)} //nolint:exhaustruct

	updated, result, err := applyPoolSizingFromShape(context.Background(), cfg, modeEnforce, imds)
	if err != nil {
		t.Fatalf("applyPoolSizingFromShape returned error: %v", err)
	}

	if !result.applied {
		t.Fatal("expected sizing to be applied")
	}

	if updated.Pool.Workers != 7 {
		t.Fatalf("expected worker count to follow OCPUs, got %d", updated.Pool.Workers)
	}

	if imds.shapeCalls != 1 {
		t.Fatalf("expected one ShapeConfig call, got %d", imds.shapeCalls)
	}
}

func TestApplyPoolSizingFromShapeClampsAndSkipsOffline(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.Pool.AutoSizeFromShape = true
	cfg.OCI.Offline = true

	imds := &stubIMDSClient{shape: stubShapeConfig(64, 0)} //nolint:exhaustruct

	updated, result, err := applyPoolSizingFromShape(context.Background(), cfg, modeEnforce, imds)
	if err != nil {
		t.Fatalf("applyPoolSizingFromShape returned error: %v", err)
	}

	if result.applied {
		t.Fatal("expected offline mode to skip sizing")
	}

	if updated.Pool.Workers != cfg.Pool.Workers {
		t.Fatalf(
			"expected worker count to remain %d, got %d",
			cfg.Pool.Workers,
			updated.Pool.Workers,
		)
	}

	if imds.shapeCalls != 0 {
		t.Fatalf("expected no ShapeConfig calls offline, got %d", imds.shapeCalls)
	}
}

func TestApplyPoolSizingFromShapeErrors(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.Pool.AutoSizeFromShape = true

	imds := &stubIMDSClient{ //nolint:exhaustruct
		shapeErr: errShapeLookupFailed,
	}

	_, _, err := applyPoolSizingFromShape(context.Background(), cfg, modeEnforce, imds)
	if err == nil {
		t.Fatal("expected error when shape lookup fails")
	}
}

func TestDeriveWorkerCountFromOCPUs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		ocpus    float64
		fallback int
		want     int
		applied  bool
		capped   bool
	}{
		{name: "fallback", ocpus: 0, fallback: 3, want: 3, applied: false, capped: false},
		{name: "negative", ocpus: -8, fallback: 4, want: 4, applied: false, capped: false},
		{name: "belowOne", ocpus: 0.25, fallback: 2, want: 1, applied: true, capped: false},
		{name: "fractionalCeil", ocpus: 2.75, fallback: 2, want: 3, applied: true, capped: false},
		{
			name:     "maxCap",
			ocpus:    64,
			fallback: 2,
			want:     maxAutoSizedWorkers,
			applied:  true,
			capped:   true,
		},
	}

	for _, tc := range testCases {
		testCase := tc

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assertDeriveWorkerCountFromOCPUs(
				t,
				testCase.ocpus,
				testCase.fallback,
				testCase.want,
				testCase.applied,
				testCase.capped,
			)
		})
	}
}

func TestDeriveWorkerCountFromOCPUSBounds(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		ocpus    float64
		fallback int
		want     int
		applied  bool
		capped   bool
	}{
		{
			name:     "zeroOCPUsFallback",
			ocpus:    0,
			fallback: 5,
			want:     5,
			applied:  false,
			capped:   false,
		},
		{
			name:     "negativeOCPUsFallback",
			ocpus:    -3,
			fallback: 6,
			want:     6,
			applied:  false,
			capped:   false,
		},
		{
			name:     "fractionalBelowMinWorkers",
			ocpus:    0.4,
			fallback: 2,
			want:     minAutoSizedWorkers,
			applied:  true,
			capped:   false,
		},
		{
			name:     "aboveMaxWorkers",
			ocpus:    128,
			fallback: 1,
			want:     maxAutoSizedWorkers,
			applied:  true,
			capped:   true,
		},
	}

	for _, tc := range testCases {
		testCase := tc

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assertDeriveWorkerCountFromOCPUs(
				t,
				testCase.ocpus,
				testCase.fallback,
				testCase.want,
				testCase.applied,
				testCase.capped,
			)
		})
	}
}

func assertDeriveWorkerCountFromOCPUs(
	t *testing.T,
	ocpus float64,
	fallback int,
	wantWorkers int,
	wantApplied bool,
	wantCapped bool,
) {
	t.Helper()

	got, applied, capped := deriveWorkerCountFromOCPUs(ocpus, fallback)
	if got != wantWorkers || applied != wantApplied || capped != wantCapped {
		t.Fatalf(
			"deriveWorkerCountFromOCPUs(%v, %d) = (%d,%t,%t), want (%d,%t,%t)",
			ocpus,
			fallback,
			got,
			applied,
			capped,
			wantWorkers,
			wantApplied,
			wantCapped,
		)
	}
}

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
