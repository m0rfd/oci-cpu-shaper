//nolint:testpackage // tests require access to unexported config helpers.
package runtimeconfig

import (
	"errors"
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
	stubRegion              = "us-ashburn-1"
)

func TestLoadConfigUsesDefaultsWhenFileMissing(t *testing.T) {
	t.Parallel()

	cfg, err := Load("./testdata/missing.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	defaults := adaptDefault()

	assertFloatEqual(t, "targetStart", cfg.Controller.TargetStart, defaults.TargetStart)
	assertStringEqual(t, "httpBind", cfg.HTTP.Bind, ":9108")
	assertDurationEqual(t, "estimatorInterval", cfg.Estimator.Interval, time.Second)
	assertIntEqual(t, "poolWorkers", cfg.Pool.Workers, 2)
	assertBoolEqual(t, "offline", cfg.OCI.Offline, false)
	assertStringEqual(t, "region", cfg.OCI.Region, "")
	assertFloatEqual(
		t,
		"suppressThreshold",
		cfg.Controller.SuppressThreshold,
		defaults.SuppressThreshold,
	)
	assertFloatEqual(t, "suppressResume", cfg.Controller.SuppressResume, defaults.SuppressResume)
	assertFloatEqual(
		t,
		"suppressRunnableThreshold",
		cfg.Controller.SuppressRunnableThreshold,
		defaults.SuppressRunnableThreshold,
	)
	assertFloatEqual(
		t,
		"suppressRunnableResume",
		cfg.Controller.SuppressRunnableResume,
		defaults.SuppressRunnableResume,
	)
	assertFloatEqual(t, "poolPauseThreshold", cfg.Pool.PauseThreshold, defaults.SuppressThreshold)
	assertFloatEqual(t, "poolResumeThreshold", cfg.Pool.ResumeThreshold, defaults.SuppressResume)
	assertIntEqual(
		t,
		"relaxedConfirmations",
		cfg.Controller.RelaxedConfirmations,
		defaults.RelaxedConfirmations,
	)
}

func TestLoadConfigSamplesLoad(t *testing.T) {
	t.Parallel()

	samples := []struct {
		name string
		path string
	}{
		{name: "mode-a", path: filepath.Join("..", "..", "configs", "mode-a.yaml")},
		{name: "mode-b", path: filepath.Join("..", "..", "configs", "mode-b.yaml")},
	}

	for _, sample := range samples {
		t.Run(sample.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := Load(sample.path)
			if err != nil {
				t.Fatalf("Load returned error for %s: %v", sample.path, err)
			}

			if strings.TrimSpace(cfg.HTTP.Bind) == "" {
				t.Fatalf("expected %s to enable HTTP listener", sample.name)
			}
		})
	}
}

func TestLoadConfigAppliesFileOverrides(t *testing.T) {
	t.Parallel()

	cfg, err := Load(filepath.Join("testdata", "config.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
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
	assertFloatEqual(t, "suppressRunnableThreshold", cfg.Controller.SuppressRunnableThreshold, 1.05)
	assertFloatEqual(t, "suppressRunnableResume", cfg.Controller.SuppressRunnableResume, 0.8)
	assertFloatEqual(t, "poolPauseThreshold", cfg.Pool.PauseThreshold, 0.88)
	assertFloatEqual(t, "poolResumeThreshold", cfg.Pool.ResumeThreshold, 0.44)
	assertIntEqual(t, "relaxedConfirmations", cfg.Controller.RelaxedConfirmations, 3)
}

//nolint:paralleltest // uses t.Setenv for multiple overrides
func TestLoadConfigAppliesEnvOverrides(t *testing.T) {
	overrides := map[string]string{
		envTargetStart:               "0.30",
		envTargetMin:                 "0.20",
		envStepUp:                    "0.05",
		envSlowInterval:              "2h",
		envRelaxedInterval:           "12h",
		envRelaxedConfirmations:      "5",
		envFastInterval:              "250ms",
		envPoolWorkers:               "4",
		envPoolPauseThreshold:        "0.81",
		envPoolResumeThreshold:       "0.49",
		envHTTPBind:                  " :9300 ",
		envCompartmentID:             " " + testCompartmentOverride + " ",
		envInstanceID:                " ocid1.instance.oc1..override ",
		envOCIRegion:                 " " + testRegionOverride + " ",
		envOCIOffline:                "true",
		envSuppressThreshold:         "0.88",
		envSuppressResume:            "0.51",
		envSuppressRunnableThreshold: "1.3",
		envSuppressRunnableResume:    "0.7",
	}

	for key, value := range overrides {
		t.Setenv(key, value)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	assertFloatEqual(t, "targetStart", cfg.Controller.TargetStart, 0.30)
	assertFloatEqual(t, "targetMin", cfg.Controller.TargetMin, 0.20)
	assertFloatEqual(t, "stepUp", cfg.Controller.StepUp, 0.05)
	assertDurationEqual(t, "interval", cfg.Controller.Interval, 2*time.Hour)
	assertDurationEqual(t, "relaxedInterval", cfg.Controller.RelaxedInterval, 12*time.Hour)
	assertIntEqual(t, "relaxedConfirmations", cfg.Controller.RelaxedConfirmations, 5)
	assertFloatEqual(t, "suppressThreshold", cfg.Controller.SuppressThreshold, 0.88)
	assertFloatEqual(t, "suppressResume", cfg.Controller.SuppressResume, 0.51)
	assertFloatEqual(
		t,
		"suppressRunnableThreshold",
		cfg.Controller.SuppressRunnableThreshold,
		1.3,
	)
	assertFloatEqual(t, "suppressRunnableResume", cfg.Controller.SuppressRunnableResume, 0.7)
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

func TestLoadConfigAllowsClearingHTTPBindViaEnv(t *testing.T) {
	t.Setenv(envHTTPBind, "   ")

	cfg, err := Load(filepath.Join("testdata", "config.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.HTTP.Bind != "" {
		t.Fatalf("expected HTTP bind to be cleared by env override, got %q", cfg.HTTP.Bind)
	}
}

func TestLoadConfigRejectsTargetsExceedingSuppressThreshold(t *testing.T) {
	t.Setenv(envSuppressThreshold, "0.25")
	t.Setenv(envSuppressResume, "0.24")

	_, err := Load("")
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

	_, err := Load("")
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

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
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

	overrideCfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
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

	_, err = Load(path)
	if err == nil {
		t.Fatal("expected error for malformed yaml")
	}
}
