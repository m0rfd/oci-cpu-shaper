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

type poolSizingEdgeCase struct {
	name            string
	ocpus           float64
	initialWorkers  int
	expectedWorkers int
	expectedApplied bool
	expectedCapped  bool
}

type autosizeSkipCase struct {
	name    string
	mode    string
	offline bool
}

func TestApplyPoolSizingFromShapeSkipsWhenUnavailableWithoutIMDS(t *testing.T) {
	t.Parallel()

	testCases := []autosizeSkipCase{
		{
			name:    "noopModeSkipsWithoutIMDS",
			mode:    modeNoop,
			offline: false,
		},
		{
			name:    "offlineSkipsWithoutIMDS",
			mode:    modeEnforce,
			offline: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := runtimeconfig.Default()
			cfg.Pool.AutoSizeFromShape = true
			cfg.Pool.Workers = 4
			cfg.OCI.Offline = testCase.offline

			updated, result, err := applyPoolSizingFromShape(
				context.Background(),
				cfg,
				testCase.mode,
				nil,
			)
			if err != nil {
				t.Fatalf("applyPoolSizingFromShape returned error: %v", err)
			}

			if result.applied {
				t.Fatal("expected sizing to be skipped")
			}

			if updated.Pool.Workers != cfg.Pool.Workers {
				t.Fatalf(
					"expected worker count to remain %d, got %d",
					cfg.Pool.Workers,
					updated.Pool.Workers,
				)
			}

			if result.workers != cfg.Pool.Workers {
				t.Fatalf("expected worker result %d, got %d", cfg.Pool.Workers, result.workers)
			}
		})
	}
}

func TestApplyPoolSizingFromShapeIMDSEdgeOCPUs(t *testing.T) {
	t.Parallel()

	testCases := []poolSizingEdgeCase{
		{
			name:            "zeroFallsBack",
			ocpus:           0,
			initialWorkers:  4,
			expectedWorkers: 4,
			expectedApplied: false,
			expectedCapped:  false,
		},
		{
			name:            "subOneRoundsToMinimum",
			ocpus:           0.4,
			initialWorkers:  3,
			expectedWorkers: minAutoSizedWorkers,
			expectedApplied: true,
			expectedCapped:  true,
		},
		{
			name:            "capsAboveMaximum",
			ocpus:           48,
			initialWorkers:  2,
			expectedWorkers: maxAutoSizedWorkers,
			expectedApplied: true,
			expectedCapped:  true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runPoolSizingEdgeCase(t, testCase)
		})
	}
}

func runPoolSizingEdgeCase(t *testing.T, testCase poolSizingEdgeCase) {
	t.Helper()

	cfg := runtimeconfig.Default()
	cfg.Pool.AutoSizeFromShape = true
	cfg.Pool.Workers = testCase.initialWorkers

	imds := &stubIMDSClient{shape: stubShapeConfig(testCase.ocpus, 0)} //nolint:exhaustruct

	updated, result, err := applyPoolSizingFromShape(context.Background(), cfg, modeEnforce, imds)
	if err != nil {
		t.Fatalf("applyPoolSizingFromShape returned error: %v", err)
	}

	if result.applied != testCase.expectedApplied {
		t.Fatalf("expected applied=%t, got %t", testCase.expectedApplied, result.applied)
	}

	if result.capped != testCase.expectedCapped {
		t.Fatalf("expected capped=%t, got %t", testCase.expectedCapped, result.capped)
	}

	if result.workers != testCase.expectedWorkers {
		t.Fatalf("expected worker result %d, got %d", testCase.expectedWorkers, result.workers)
	}

	if updated.Pool.Workers != testCase.expectedWorkers {
		t.Fatalf(
			"expected config workers %d, got %d",
			testCase.expectedWorkers,
			updated.Pool.Workers,
		)
	}

	if result.shapeOCPUs != testCase.ocpus {
		t.Fatalf("expected shape OCPUs %.1f, got %.1f", testCase.ocpus, result.shapeOCPUs)
	}

	if imds.shapeCalls != 1 {
		t.Fatalf("expected one ShapeConfig call, got %d", imds.shapeCalls)
	}
}

func TestApplyPoolSizingFromShapeSkipsAutosizingWhenUnavailable(t *testing.T) {
	t.Parallel()

	testCases := []autosizeSkipCase{
		{
			name:    "noopModeSkips",
			mode:    modeNoop,
			offline: false,
		},
		{
			name:    "offlineSkips",
			mode:    modeEnforce,
			offline: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runAutosizingSkipCase(t, testCase)
		})
	}
}

func TestApplyPoolSizingFromShapeRequiresIMDSWhenAutosizingEnabled(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.Pool.AutoSizeFromShape = true

	updated, result, err := applyPoolSizingFromShape(context.Background(), cfg, modeEnforce, nil)
	if err == nil {
		t.Fatal("expected error when autosizing without IMDS client")
	}

	if !errors.Is(err, errControllerIMDSRequired) {
		t.Fatalf("expected errControllerIMDSRequired, got %v", err)
	}

	if result.applied {
		t.Fatal("expected sizing not to be applied when IMDS is missing")
	}

	if updated.Pool.Workers != cfg.Pool.Workers {
		t.Fatalf(
			"expected worker count to remain %d, got %d",
			cfg.Pool.Workers,
			updated.Pool.Workers,
		)
	}

	if result.workers != cfg.Pool.Workers {
		t.Fatalf("expected worker result %d, got %d", cfg.Pool.Workers, result.workers)
	}
}

func runAutosizingSkipCase(t *testing.T, testCase autosizeSkipCase) {
	t.Helper()

	cfg := runtimeconfig.Default()
	cfg.Pool.AutoSizeFromShape = true
	cfg.Pool.Workers = 6
	cfg.OCI.Offline = testCase.offline

	imds := &stubIMDSClient{shape: stubShapeConfig(32, 0)} //nolint:exhaustruct

	updated, result, err := applyPoolSizingFromShape(context.Background(), cfg, testCase.mode, imds)
	if err != nil {
		t.Fatalf("applyPoolSizingFromShape returned error: %v", err)
	}

	if result.applied {
		t.Fatal("expected sizing to be skipped")
	}

	if result.capped {
		t.Fatal("expected capped flag to remain false when sizing is skipped")
	}

	if result.workers != cfg.Pool.Workers {
		t.Fatalf("expected worker result %d, got %d", cfg.Pool.Workers, result.workers)
	}

	if updated.Pool.Workers != cfg.Pool.Workers {
		t.Fatalf("expected config workers %d, got %d", cfg.Pool.Workers, updated.Pool.Workers)
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
