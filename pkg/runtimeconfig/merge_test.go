//nolint:testpackage // tests require access to unexported config helpers.
package runtimeconfig

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMergeRuntimeConfigFileIgnoresMissingFile(t *testing.T) {
	t.Parallel()

	cfg := Default()
	copyCfg := cfg

	err := mergeRuntimeConfigFile(&cfg, filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("mergeRuntimeConfigFile returned error: %v", err)
	}

	if cfg != copyCfg {
		t.Fatalf("expected config to remain unchanged when file is missing")
	}
}

func TestMergeRuntimeConfigFileAppliesOverrides(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Pool.Workers = 10

	path := writeTempConfig(t, mergeOverridesFixture())

	err := mergeRuntimeConfigFile(&cfg, path)
	if err != nil {
		t.Fatalf("mergeRuntimeConfigFile returned error: %v", err)
	}

	assertMergeOverrides(t, cfg)
}

func mergeOverridesFixture() string {
	return `controller:
  targetStart: 0.42
  targetMin: 0.40
  targetMax: 0.66
  stepUp: 0.04
  stepDown: 0.02
  interval: 15m
  relaxedInterval: 6h
  fallbackTarget: 0.3
  relaxedThreshold: 0.25
  relaxedConfirmations: 4
  suppressThreshold: 0.91
  suppressResume: 0.61
  suppressRunnableThreshold: 1.3
  suppressRunnableResume: 1.0
  suppressSmoothingSamples: 7
  goalLow: 0.44
  goalHigh: 0.54
estimator:
  interval: 750ms
pool:
  workers: 3
  quantum: 33ms
  pauseThreshold: 0.8
  resumeThreshold: 0.5
  runnableGuard: 1.25
http:
  bind: " :9999 "
oci:
  compartmentId: ocid1.compartment.oc1..merge
  region: us-phoenix-1
  instanceId: ocid1.instance.oc1..merge
  offline: true
`
}

func assertMergeOverrides(t *testing.T, cfg Config) {
	t.Helper()

	assertFloatEqual(t, "targetStart", cfg.Controller.TargetStart, 0.42)
	assertFloatEqual(t, "targetMin", cfg.Controller.TargetMin, 0.40)
	assertFloatEqual(t, "targetMax", cfg.Controller.TargetMax, 0.66)
	assertFloatEqual(t, "stepUp", cfg.Controller.StepUp, 0.04)
	assertFloatEqual(t, "stepDown", cfg.Controller.StepDown, 0.02)
	assertFloatEqual(t, "fallbackTarget", cfg.Controller.FallbackTarget, 0.3)
	assertFloatEqual(t, "goalLow", cfg.Controller.GoalLow, 0.44)
	assertFloatEqual(t, "goalHigh", cfg.Controller.GoalHigh, 0.54)
	assertDurationEqual(t, "interval", cfg.Controller.Interval, 15*time.Minute)
	assertDurationEqual(t, "relaxedInterval", cfg.Controller.RelaxedInterval, 6*time.Hour)
	assertFloatEqual(t, "relaxedThreshold", cfg.Controller.RelaxedThreshold, 0.25)
	assertIntEqual(t, "relaxedConfirmations", cfg.Controller.RelaxedConfirmations, 4)
	assertFloatEqual(t, "suppressThreshold", cfg.Controller.SuppressThreshold, 0.91)
	assertFloatEqual(t, "suppressResume", cfg.Controller.SuppressResume, 0.61)
	assertFloatEqual(t, "suppressRunnableThreshold", cfg.Controller.SuppressRunnableThreshold, 1.3)
	assertFloatEqual(t, "suppressRunnableResume", cfg.Controller.SuppressRunnableResume, 1.0)
	assertIntEqual(t, "suppressSmoothingSamples", cfg.Controller.SuppressSmoothingSamples, 7)
	assertDurationEqual(t, "estimatorInterval", cfg.Estimator.Interval, 750*time.Millisecond)
	assertIntEqual(t, "poolWorkers", cfg.Pool.Workers, 3)
	assertDurationEqual(t, "poolQuantum", cfg.Pool.Quantum, 33*time.Millisecond)
	assertFloatEqual(t, "poolPauseThreshold", cfg.Pool.PauseThreshold, 0.8)
	assertFloatEqual(t, "poolResumeThreshold", cfg.Pool.ResumeThreshold, 0.5)
	assertFloatEqual(t, "poolRunnableGuard", cfg.Pool.RunnableGuard, 1.25)
	assertStringEqual(t, "httpBind", cfg.HTTP.Bind, ":9999")
	assertStringEqual(t, "compartmentId", cfg.OCI.CompartmentID, "ocid1.compartment.oc1..merge")
	assertStringEqual(t, "region", cfg.OCI.Region, "us-phoenix-1")
	assertStringEqual(t, "instanceId", cfg.OCI.InstanceID, "ocid1.instance.oc1..merge")
	assertBoolEqual(t, "offline", cfg.OCI.Offline, true)
}
