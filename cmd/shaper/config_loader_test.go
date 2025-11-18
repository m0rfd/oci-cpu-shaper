package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/adapt"
)

const (
	testCompartmentOverride = "ocid1.compartment.oc1..override"
	testRegionOverride      = "us-sanjose-1"
)

func TestLoadConfigUsesDefaultsWhenFileMissing(t *testing.T) {
	t.Parallel()

	cfg, err := loadConfig("./testdata/missing.yaml")
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	defaults := adaptDefault()

	assertFloatEqual(t, "targetStart", cfg.Controller.TargetStart, defaults.TargetStart)
	assertStringEqual(t, "httpBind", cfg.HTTP.Bind, ":9108")
	assertDurationEqual(t, "estimatorInterval", cfg.Estimator.Interval, time.Second)
	assertBoolEqual(t, "offline", cfg.OCI.Offline, false)
	assertStringEqual(t, "region", cfg.OCI.Region, "")
	assertFloatEqual(
		t,
		"suppressThreshold",
		cfg.Controller.SuppressThreshold,
		defaults.SuppressThreshold,
	)
	assertFloatEqual(t, "suppressResume", cfg.Controller.SuppressResume, defaults.SuppressResume)
	assertFloatEqual(t, "poolPauseThreshold", cfg.Pool.PauseThreshold, defaults.SuppressThreshold)
	assertFloatEqual(t, "poolResumeThreshold", cfg.Pool.ResumeThreshold, defaults.SuppressResume)
}

func TestLoadConfigSamplesMatchDefaults(t *testing.T) {
	t.Parallel()

	defaults := adaptDefault()

	samples := []struct {
		name string
		path string
	}{
		{name: "mode-a", path: filepath.Join("..", "configs", "mode-a.yaml")},
		{name: "mode-b", path: filepath.Join("..", "configs", "mode-b.yaml")},
	}

	for _, sample := range samples {
		t.Run(sample.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := loadConfig(sample.path)
			if err != nil {
				t.Fatalf("loadConfig returned error for %s: %v", sample.path, err)
			}

			assertFloatEqual(t, "goalLow", cfg.Controller.GoalLow, defaults.GoalLow)
			assertFloatEqual(t, "goalHigh", cfg.Controller.GoalHigh, defaults.GoalHigh)
			assertFloatEqual(
				t,
				"suppressThreshold",
				cfg.Controller.SuppressThreshold,
				defaults.SuppressThreshold,
			)
			assertFloatEqual(
				t,
				"suppressResume",
				cfg.Controller.SuppressResume,
				defaults.SuppressResume,
			)
			assertStringEqual(t, "httpBind", cfg.HTTP.Bind, ":9108")
			assertDurationEqual(t, "estimatorInterval", cfg.Estimator.Interval, time.Second)
			assertFloatEqual(
				t,
				"poolPauseThreshold",
				cfg.Pool.PauseThreshold,
				defaults.SuppressThreshold,
			)
			assertFloatEqual(
				t,
				"poolResumeThreshold",
				cfg.Pool.ResumeThreshold,
				defaults.SuppressResume,
			)
		})
	}
}

func TestLoadConfigAppliesFileOverrides(t *testing.T) {
	t.Parallel()

	cfg, err := loadConfig(filepath.Join("testdata", "config.yaml"))
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	assertFloatEqual(t, "targetStart", cfg.Controller.TargetStart, 0.26)
	assertDurationEqual(t, "controllerInterval", cfg.Controller.Interval, 30*time.Minute)
	assertIntEqual(t, "poolWorkers", cfg.Pool.Workers, 2)
	assertStringEqual(t, "httpBind", cfg.HTTP.Bind, ":9200")
	assertStringEqual(
		t,
		"compartmentID",
		cfg.OCI.CompartmentID,
		"ocid1.compartment.oc1..exampleuniqueID",
	)
	assertStringEqual(t, "instanceID", cfg.OCI.InstanceID, "ocid1.instance.oc1..config")
	assertStringEqual(t, "region", cfg.OCI.Region, stubRegion)
	assertFloatEqual(t, "suppressThreshold", cfg.Controller.SuppressThreshold, 0.9)
	assertFloatEqual(t, "suppressResume", cfg.Controller.SuppressResume, 0.6)
	assertFloatEqual(t, "poolPauseThreshold", cfg.Pool.PauseThreshold, 0.88)
	assertFloatEqual(t, "poolResumeThreshold", cfg.Pool.ResumeThreshold, 0.44)
}

func TestLoadConfigAppliesEnvOverrides(t *testing.T) {
	t.Setenv(envTargetStart, "0.33")
	t.Setenv(envTargetMin, "0.20")
	t.Setenv(envStepUp, "0.05")
	t.Setenv(envSlowInterval, "2h")
	t.Setenv(envRelaxedInterval, "12h")
	t.Setenv(envFastInterval, "250ms")
	t.Setenv(envPoolWorkers, "4")
	t.Setenv(envPoolPauseThreshold, "0.81")
	t.Setenv(envPoolResumeThreshold, "0.49")
	t.Setenv(envHTTPBind, " :9300 ")
	t.Setenv(envCompartmentID, " "+testCompartmentOverride+" ")
	t.Setenv(envInstanceID, " ocid1.instance.oc1..override ")
	t.Setenv(envOCIRegion, " "+testRegionOverride+" ")
	t.Setenv(envOCIOffline, "true")
	t.Setenv(envSuppressThreshold, "0.88")
	t.Setenv(envSuppressResume, "0.51")

	cfg, err := loadConfig("")
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	assertFloatEqual(t, "targetStart", cfg.Controller.TargetStart, 0.33)
	assertFloatEqual(t, "targetMin", cfg.Controller.TargetMin, 0.20)
	assertFloatEqual(t, "stepUp", cfg.Controller.StepUp, 0.05)
	assertDurationEqual(t, "interval", cfg.Controller.Interval, 2*time.Hour)
	assertDurationEqual(t, "relaxedInterval", cfg.Controller.RelaxedInterval, 12*time.Hour)
	assertFloatEqual(t, "suppressThreshold", cfg.Controller.SuppressThreshold, 0.88)
	assertFloatEqual(t, "suppressResume", cfg.Controller.SuppressResume, 0.51)
	assertFloatEqual(t, "poolPauseThreshold", cfg.Pool.PauseThreshold, 0.81)
	assertFloatEqual(t, "poolResumeThreshold", cfg.Pool.ResumeThreshold, 0.49)
	assertDurationEqual(t, "estimatorInterval", cfg.Estimator.Interval, 250*time.Millisecond)
	assertIntEqual(t, "workers", cfg.Pool.Workers, 4)
	assertStringEqual(t, "httpBind", cfg.HTTP.Bind, ":9300")
	assertStringEqual(t, "compartmentID", cfg.OCI.CompartmentID, testCompartmentOverride)
	assertStringEqual(t, "region", cfg.OCI.Region, testRegionOverride)
	assertStringEqual(t, "instanceID", cfg.OCI.InstanceID, "ocid1.instance.oc1..override")
	assertBoolEqual(t, "offline", cfg.OCI.Offline, true)
}

func TestLoadConfigRejectsTargetsExceedingSuppressThreshold(t *testing.T) {
	t.Setenv(envSuppressThreshold, "0.35")
	t.Setenv(envSuppressResume, "0.34")

	_, err := loadConfig("")
	if err == nil {
		t.Fatal("expected validation error when suppressThreshold is below target values")
	}

	if !errors.Is(err, adapt.ErrInvalidConfig) {
		t.Fatalf("expected adapt.ErrInvalidConfig, got %v", err)
	}

	if !strings.Contains(err.Error(), "controller.targetMax") {
		t.Fatalf("expected error to reference controller.targetMax, got %v", err)
	}
}

func TestLoadConfigRejectsTargetsExceedingSuppressResume(t *testing.T) {
	t.Setenv(envSuppressResume, "0.10")

	_, err := loadConfig("")
	if err == nil {
		t.Fatal("expected validation error when suppressResume is below target values")
	}

	if !errors.Is(err, adapt.ErrInvalidConfig) {
		t.Fatalf("expected adapt.ErrInvalidConfig, got %v", err)
	}

	if !strings.Contains(err.Error(), "controller.targetStart") {
		t.Fatalf("expected error to reference controller.targetStart, got %v", err)
	}
}

//nolint:paralleltest // manipulates lookupEnv global for deterministic overrides
func TestLoadConfigAppliesOfflineFileOverride(t *testing.T) {
	path := filepath.Join("testdata", "offline.yaml")

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	if !cfg.OCI.Offline {
		t.Fatal("expected offline flag to be enabled from file config")
	}

	assertStringEqual(t, "instanceID", cfg.OCI.InstanceID, "ocid1.instance.oc1..offline")

	expectedOverride := "ocid1.instance.oc1..override"
	origLookupEnv := lookupEnv

	t.Cleanup(func() { lookupEnv = origLookupEnv })

	lookupEnv = func(key string) (string, bool) {
		if key == envInstanceID {
			return " " + expectedOverride + " ", true
		}

		return origLookupEnv(key)
	}

	overrideCfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	if !overrideCfg.OCI.Offline {
		t.Fatal("expected offline flag to remain enabled after env override")
	}

	assertStringEqual(t, "instanceID", overrideCfg.OCI.InstanceID, expectedOverride)
}

func TestLoadConfigReturnsDecodeError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "bad.yaml")

	err := os.WriteFile(path, []byte("controller: ["), testConfigFilePerm)
	if err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	_, err = loadConfig(path)
	if err == nil {
		t.Fatal("expected error for malformed yaml")
	}
}

func TestLoadRuntimeConfigOrExitReturnsParseCodeOnValidationError(t *testing.T) {
	t.Parallel()

	var deps runDeps

	deps.loadConfig = func(string) (runtimeConfig, error) {
		return runtimeConfig{}, fmt.Errorf("wrap: %w", adapt.ErrInvalidConfig)
	}

	var stderr bytes.Buffer

	_, exitCode, loaded := loadRuntimeConfigOrExit(deps, "", &stderr)
	if loaded {
		t.Fatal("expected loadRuntimeConfigOrExit to report failure")
	}

	if exitCode != exitCodeParseError {
		t.Fatalf("expected parse error exit code %d, got %d", exitCodeParseError, exitCode)
	}

	if output := stderr.String(); !strings.Contains(output, "failed to load configuration") {
		t.Fatalf("expected diagnostic output, got %q", output)
	}
}
