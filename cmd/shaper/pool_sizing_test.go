package main

import (
	"context"
	"errors"
	"testing"

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

func TestApplyPoolSizingFromShapeHandlesNonPositiveOCPUs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		ocpus         float64
		configWorkers int
	}{
		{name: "zero", ocpus: 0, configWorkers: 4},
		{name: "negative", ocpus: -2.5, configWorkers: 6},
	}

	for _, tc := range testCases {
		testCase := tc

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := runtimeconfig.Default()
			cfg.Pool.AutoSizeFromShape = true
			cfg.Pool.Workers = testCase.configWorkers

			imds := &stubIMDSClient{shape: stubShapeConfig(testCase.ocpus, 0)} //nolint:exhaustruct

			updated, result, err := applyPoolSizingFromShape(
				context.Background(),
				cfg,
				modeEnforce,
				imds,
			)
			if err != nil {
				t.Fatalf("applyPoolSizingFromShape returned error: %v", err)
			}

			if result.applied {
				t.Fatal("expected sizing to be skipped when OCPUs are non-positive")
			}

			if updated.Pool.Workers != cfg.Pool.Workers {
				t.Fatalf(
					"expected worker count to remain %d, got %d",
					cfg.Pool.Workers,
					updated.Pool.Workers,
				)
			}

			if imds.shapeCalls != 1 {
				t.Fatalf("expected one ShapeConfig call, got %d", imds.shapeCalls)
			}
		})
	}
}

func TestApplyPoolSizingFromShapeSmallOCPUs(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.Pool.AutoSizeFromShape = true
	cfg.Pool.Workers = 2

	imds := &stubIMDSClient{shape: stubShapeConfig(0.5, 0)} //nolint:exhaustruct

	updated, result, err := applyPoolSizingFromShape(context.Background(), cfg, modeEnforce, imds)
	if err != nil {
		t.Fatalf("applyPoolSizingFromShape returned error: %v", err)
	}

	if !result.applied {
		t.Fatal("expected sizing to be applied for fractional OCPU counts")
	}

	if updated.Pool.Workers != 1 {
		t.Fatalf(
			"expected worker count to ceil fractional OCPUs to 1, got %d",
			updated.Pool.Workers,
		)
	}

	if result.capped {
		t.Fatal("expected small OCPU counts not to report capping")
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

func TestApplyPoolSizingFromShapeSkipsInNoopMode(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.Pool.AutoSizeFromShape = true
	cfg.Pool.Workers = 5

	imds := &stubIMDSClient{shape: stubShapeConfig(8, 0)} //nolint:exhaustruct

	updated, result, err := applyPoolSizingFromShape(context.Background(), cfg, modeNoop, imds)
	if err != nil {
		t.Fatalf("applyPoolSizingFromShape returned error: %v", err)
	}

	if result.applied {
		t.Fatal("expected noop mode to skip sizing")
	}

	if updated.Pool.Workers != cfg.Pool.Workers {
		t.Fatalf("expected worker count to remain unchanged, got %d", updated.Pool.Workers)
	}

	if imds.shapeCalls != 0 {
		t.Fatalf("expected shape lookup to be skipped, got %d calls", imds.shapeCalls)
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

	if imds.shapeCalls != 1 {
		t.Fatalf("expected ShapeConfig call despite error, got %d", imds.shapeCalls)
	}
}

func TestApplyPoolSizingFromShapeCapsWorkers(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.Pool.AutoSizeFromShape = true
	cfg.Pool.Workers = 2

	imds := &stubIMDSClient{shape: stubShapeConfig(64, 0)} //nolint:exhaustruct

	updated, result, err := applyPoolSizingFromShape(context.Background(), cfg, modeEnforce, imds)
	if err != nil {
		t.Fatalf("applyPoolSizingFromShape returned error: %v", err)
	}

	if !result.applied {
		t.Fatal("expected sizing to be applied")
	}

	if !result.capped {
		t.Fatal("expected sizing to be capped at the maximum workers")
	}

	if updated.Pool.Workers != maxAutoSizedWorkers {
		t.Fatalf(
			"expected workers to be capped at %d, got %d",
			maxAutoSizedWorkers,
			updated.Pool.Workers,
		)
	}

	if imds.shapeCalls != 1 {
		t.Fatalf("expected one ShapeConfig call, got %d", imds.shapeCalls)
	}
}
