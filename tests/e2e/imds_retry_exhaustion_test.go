//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"oci-cpu-shaper/internal/e2eclient"
	interne2e "oci-cpu-shaper/tests/internal/e2e"
)

func TestIMDSRetryBudgetExhaustionBlocksFallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repoRoot := interne2e.RepositoryRoot(t)
	binary := interne2e.BuildShaperBinary(t, repoRoot, "e2e")

	imdsServer := interne2e.StartIMDSHandlerServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	})

	monitoring := interne2e.StartMonitoringServer(t, nil)

	configPath := writeConfig(t, "imds-retry-exhaustion.yaml", `controller:
  interval: 1s
  relaxedInterval: 2s
estimator:
  interval: 200ms
pool:
  workers: 1
  quantum: 150ms
http:
  bind: ""
oci:
  fallbackTarget: 0.22
`)

	cmd := exec.CommandContext(ctx, binary, "--config", configPath, "--shutdown-after=1s", "--log-level=debug")

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.Env = append([]string{}, os.Environ()...)
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("%s=%s", imdsEndpointEnv, imdsServer.Endpoint()),
		fmt.Sprintf("%s=%s", e2eclient.MonitoringEndpointEnv, monitoring.URL()),
	)

	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected non-zero exit, got %v\n%s", err, output.String())
	}

	if exitErr.ExitCode() != exitCodeRuntimeErrorValue {
		t.Fatalf("expected runtime error exit code %d, got %d", exitCodeRuntimeErrorValue, exitErr.ExitCode())
	}

	if !bytes.Contains(output.Bytes(), []byte("failed to resolve oci metadata")) {
		t.Fatalf("expected metadata resolution failure to surface, output:\n%s", output.String())
	}

	const imdsRetryBudget = 3
	const expectedIMDSRequests = imdsRetryBudget * 3 // compartment, canonical region, and region requests
	if requests := imdsServer.Requests(); len(requests) != expectedIMDSRequests {
		t.Fatalf("expected %d metadata attempts, got %d: %v", expectedIMDSRequests, len(requests), requests)
	}

	if requests := monitoring.Requests(); len(requests) != 0 {
		t.Fatalf("expected monitoring to remain unused, got %d requests", len(requests))
	}
}
