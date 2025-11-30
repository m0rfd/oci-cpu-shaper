package main

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/adapt"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

func TestLogControllerInitializationNilLogger(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Config{} //nolint:exhaustruct
	ctrl := &trackingController{} //nolint:exhaustruct

	logControllerInitialization(nil, cfg, ctrl, nil)

	if ctrl.modeCalled || ctrl.stateCalled || ctrl.runCalled {
		t.Fatalf(
			"unexpected controller interactions: modeCalled=%v stateCalled=%v runCalled=%v",
			ctrl.modeCalled,
			ctrl.stateCalled,
			ctrl.runCalled,
		)
	}
}

func TestLogControllerInitializationNilController(t *testing.T) {
	t.Parallel()

	logger, observed := newObservedLogger(zap.InfoLevel)

	cfg := runtimeconfig.Config{ //nolint:exhaustruct
		Pool: runtimeconfig.PoolConfig{
			Workers:           1,
			AutoSizeFromShape: true,
			Quantum:           10 * time.Millisecond,
			PauseThreshold:    0.85,
			ResumeThreshold:   0.75,
			RunnableGuard:     1.0,
		},
		Estimator: runtimeconfig.EstimatorConfig{
			Interval: 100 * time.Millisecond,
		},
	}

	logControllerInitialization(logger, cfg, nil, metricshttp.NewExporter())

	if observed.Len() != 0 {
		t.Fatalf("expected no log entries, got %+v", observed.All())
	}
}

func TestLogControllerInitializationIncludesOptionalMetadata(t *testing.T) {
	t.Parallel()

	logger, observed := newObservedLogger(zap.InfoLevel)

	cfg := runtimeconfig.Config{ //nolint:exhaustruct
		Pool: runtimeconfig.PoolConfig{
			Workers:           2,
			AutoSizeFromShape: true,
			Quantum:           50 * time.Millisecond,
			PauseThreshold:    0.90,
			ResumeThreshold:   0.70,
			RunnableGuard:     1.1,
		},
		Estimator: runtimeconfig.EstimatorConfig{
			Interval: 250 * time.Millisecond,
		},
		OCI: runtimeconfig.OCIConfig{ //nolint:exhaustruct
			CompartmentID: " " + stubCompartmentID + " ",
			Region:        "\n" + stubRegion + "\t",
		},
	}

	ctrl := &stubController{mode: modeDryRun, state: adapt.StateFallback} //nolint:exhaustruct

	logControllerInitialization(logger, cfg, ctrl, metricshttp.NewExporter())

	entry := requireSingleEntry(t, observed, zap.InfoLevel)
	if entry.Message != "controller initialized" {
		t.Fatalf("unexpected log message: %q", entry.Message)
	}

	requireLogFieldString(t, entry, "mode", modeDryRun)
	requireLogFieldString(t, entry, "controllerState", adapt.StateFallback.String())
	requireLogFieldString(t, entry, "compartmentID", stubCompartmentID)
	requireLogFieldString(t, entry, "region", stubRegion)

	if enforcing, ok := fieldBool(entry.Context, "enforcingTargets"); !ok || enforcing {
		t.Fatalf("expected enforcingTargets false for dry-run, got %v (present=%v)", enforcing, ok)
	}

	if metricsEnabled, ok := fieldBool(entry.Context, "metricsEnabled"); !ok || !metricsEnabled {
		t.Fatalf("expected metricsEnabled true, got %v (present=%v)", metricsEnabled, ok)
	}
}

type trackingController struct {
	modeCalled  bool
	stateCalled bool
	runCalled   bool
}

func (c *trackingController) Run(context.Context) error {
	c.runCalled = true

	return nil
}

func (c *trackingController) Mode() string {
	c.modeCalled = true

	return modeDryRun
}

func (c *trackingController) State() adapt.State {
	c.stateCalled = true

	return adapt.StateNormal
}

func (c *trackingController) LastError() error { return nil }

func (c *trackingController) LastEstimatorError() error { return nil }
