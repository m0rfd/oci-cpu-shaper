package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"oci-cpu-shaper/pkg/adapt"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

var errIMDSUnavailable = errors.New("imds unavailable")

//nolint:paralleltest // integration-style tests reuse shared recorders and metadata stubs.
func TestBuildAdaptiveControllerReturnsWrappedIMDSError(t *testing.T) {
	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = ""
	cfg.OCI.Region = ""
	cfg.OCI.InstanceID = ""
	cfg.OCI.Offline = false
	cfg.Pool.Workers = 1

	ctx := contextWithStubMetrics(t, newStubMetricsClient())

	imdsClient := &stubIMDSClient{compartmentErr: errIMDSUnavailable} //nolint:exhaustruct

	_, _, err := buildAdaptiveController(ctx, modeEnforce, cfg, imdsClient, nil)
	if err == nil {
		t.Fatal("expected buildAdaptiveController to fail when IMDS compartment lookup errors")
	}

	if !errors.Is(err, errIMDSUnavailable) {
		t.Fatalf("expected metadata error to be wrapped, got %v", err)
	}

	if !strings.Contains(err.Error(), "lookup compartment ocid") {
		t.Fatalf("expected compartment lookup prefix in error, got %v", err)
	}
}

//nolint:paralleltest // integration-style test exercises shared helper wiring.
func TestBuildAdaptiveControllerWrapsPoolConstructorError(t *testing.T) {
	cfg := runtimeconfig.Default()
	cfg.OCI.Offline = true
	cfg.Pool.Workers = 0

	_, _, err := buildAdaptiveController(context.Background(), modeDryRun, cfg, nil, nil)
	if err == nil {
		t.Fatal("expected buildAdaptiveController to fail when pool constructor errors")
	}

	if !strings.Contains(err.Error(), "build worker pool") {
		t.Fatalf("expected worker pool construction context, got %v", err)
	}

	if !strings.Contains(err.Error(), "worker count must be positive") {
		t.Fatalf("expected wrapped pool constructor error, got %v", err)
	}
}

func TestBuildAdaptiveControllerAppliesRecorderSettings(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.Offline = true
	cfg.Pool.Workers = 1

	recorder := new(recordingMetricsRecorder)

	controller, pool, err := buildAdaptiveController(
		context.Background(),
		modeDryRun,
		cfg,
		nil,
		recorder,
	)
	if err != nil {
		t.Fatalf("buildAdaptiveController returned error: %v", err)
	}

	if pool == nil {
		t.Fatal("expected worker pool to be initialized")
	}

	if controller == nil {
		t.Fatal("expected controller to be initialized")
	}

	assertRecorderInitialSettings(t, recorder, cfg)
}

func assertRecorderInitialSettings(
	t *testing.T,
	recorder *recordingMetricsRecorder,
	cfg runtimeconfig.Config,
) {
	t.Helper()

	callCounts := map[string]int{
		"mode":     recorder.modeCalls,
		"state":    recorder.stateCalls,
		"target":   recorder.targetCalls,
		"interval": recorder.intervalCalls,
	}

	for label, calls := range callCounts {
		if calls == 0 {
			t.Fatalf("expected recorder to capture %s field, got %+v", label, recorder)
		}
	}

	if recorder.mode != modeDryRun {
		t.Fatalf("unexpected mode recorded: %s", recorder.mode)
	}

	if recorder.state != adapt.StateFallback.String() {
		t.Fatalf("unexpected initial state recorded: %s", recorder.state)
	}

	if recorder.target != cfg.Controller.FallbackTarget {
		t.Fatalf(
			"expected recorder to capture fallback target %.2f, got %.2f",
			cfg.Controller.FallbackTarget,
			recorder.target,
		)
	}

	if recorder.interval != cfg.Controller.Interval {
		t.Fatalf(
			"expected recorder to capture interval %s, got %s",
			cfg.Controller.Interval,
			recorder.interval,
		)
	}

	if recorder.lastErrorCalls == 0 || recorder.lastError != nil {
		t.Fatalf("expected recorder last error to be set to nil, got %+v", recorder)
	}
}
