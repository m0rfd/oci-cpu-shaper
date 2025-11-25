//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/imds"
	interne2e "oci-cpu-shaper/tests/internal/e2e"
)

const (
	imdsEndpointEnv           = "OCI_CPU_SHAPER_IMDS_ENDPOINT"
	metricsFactoryFailureEnv  = "OCI_CPU_SHAPER_E2E_FAIL_METRICS_CLIENT"
	exitCodeRuntimeErrorValue = 1
)

func TestMetricsClientFactoryFailureExitsWithRuntimeError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	repoRoot := interne2e.RepositoryRoot(t)
	binary := interne2e.BuildShaperBinary(t, repoRoot, "e2e")

	imdsServer := interne2e.StartIMDSServer(t, interne2e.IMDSConfig{ //nolint:exhaustruct
		Region:          "us-test-1",
		CanonicalRegion: "us-test-1",
		InstanceID:      "ocid1.instance.oc1..failure",
		CompartmentID:   "ocid1.compartment.oc1..failure",
		Shape:           imds.ShapeConfig{OCPUs: 2, MemoryInGBs: 16},
	})

	configPath := writeRuntimeConfig(t, "factory-failure.yaml")

	cmd := exec.CommandContext( //nolint:gosec // test harness isolates command execution
		ctx,
		binary,
		"--config", configPath,
		"--shutdown-after=1s",
		"--log-level=debug",
	)

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.Env = append([]string{}, os.Environ()...)
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("%s=%s", imdsEndpointEnv, imdsServer.Endpoint()),
		fmt.Sprintf("%s=1", metricsFactoryFailureEnv),
	)

	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected non-zero exit, got %v\n%s", err, output.String())
	}

	if exitErr.ExitCode() != exitCodeRuntimeErrorValue {
		t.Fatalf("expected runtime error exit code %d, got %d", exitCodeRuntimeErrorValue, exitErr.ExitCode())
	}

	if !bytes.Contains(output.Bytes(), []byte("build monitoring client: new instance principal client: build instance principal provider: e2e: forced metrics factory failure")) {
		t.Fatalf("expected metrics factory failure to surface, output:\n%s", output.String())
	}
}

func writeRuntimeConfig(t *testing.T, name string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, name)

	contents := []byte(`controller:
  interval: 1s
  relaxedInterval: 2s
estimator:
  interval: 200ms
pool:
  workers: 1
  quantum: 150ms
http:
  bind: "127.0.0.1:0"
oci:
  fallbackTarget: 0.22
`)

	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return path
}
