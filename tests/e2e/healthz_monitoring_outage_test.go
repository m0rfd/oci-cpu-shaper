//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"oci-cpu-shaper/internal/e2eclient"
	"oci-cpu-shaper/pkg/http/status"
	"oci-cpu-shaper/pkg/imds"
	interne2e "oci-cpu-shaper/tests/internal/e2e"
)

func TestHealthzSurfacesMonitoringOutage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	repoRoot := interne2e.RepositoryRoot(t)
	binary := interne2e.BuildShaperBinary(t, repoRoot, "e2e")

	imdsServer := interne2e.StartIMDSServer(t, interne2e.IMDSConfig{
		Region:          "us-test-1",
		CanonicalRegion: "us-test-1",
		InstanceID:      "ocid1.instance.oc1..healthz",
		CompartmentID:   "ocid1.compartment.oc1..healthz",
		Shape:           imds.ShapeConfig{OCPUs: 4, MemoryInGBs: 64},
	})

	monitoring := interne2e.StartMonitoringServer(t, []interne2e.MonitoringResponse{
		{Status: http.StatusServiceUnavailable, Body: "monitoring outage"},
	})

	metricsPort := interne2e.FreePort(t)
	configPath := writeConfig(t, "healthz-monitoring.yaml", fmt.Sprintf(`
controller:
  interval: 1s
  relaxedInterval: 2s
estimator:
  interval: 200ms
pool:
  workers: 1
  quantum: 150ms
http:
  bind: "127.0.0.1:%d"
oci:
  fallbackTarget: 0.22
`, metricsPort))

	var output bytes.Buffer

	cmd := exec.CommandContext(ctx, binary, "--config", configPath, "--shutdown-after=6s", "--log-level", "debug")
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.Env = append([]string{}, os.Environ()...)
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("OCI_CPU_SHAPER_IMDS_ENDPOINT=%s", imdsServer.Endpoint()),
		fmt.Sprintf("%s=%s", e2eclient.MonitoringEndpointEnv, monitoring.URL()),
	)

	if err := cmd.Start(); err != nil {
		t.Fatalf("start shaper: %v", err)
	}

	t.Cleanup(func() {
		if cmd.Process == nil {
			return
		}

		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return
		}

		_ = cmd.Process.Kill()
	})

	metricsURL := fmt.Sprintf("http://127.0.0.1:%d/metrics", metricsPort)
	if _, err := interne2e.WaitForMetrics(ctx, metricsURL); err != nil {
		t.Fatalf("wait for metrics: %v\n%s", err, output.String())
	}

	healthURL := fmt.Sprintf("http://127.0.0.1:%d/healthz", metricsPort)
	healthCtx, healthCancel := context.WithTimeout(ctx, 5*time.Second)
	snapshot, err := waitForHealthSnapshot(healthCtx, healthURL, func(s status.Snapshot) bool {
		return s.State == "fallback" && s.LastOCIError != ""
	})
	healthCancel()
	if err != nil {
		t.Fatalf("healthz never reported fallback: %v\n%s", err, output.String())
	}

	if snapshot.EstimatorError != "" {
		t.Fatalf("expected estimator error to remain empty, got %q", snapshot.EstimatorError)
	}

	if !strings.Contains(strings.ToLower(snapshot.LastOCIError), "monitoring outage") {
		t.Fatalf("expected monitoring outage to appear in health response: %+v", snapshot)
	}

	requests := monitoring.Requests()
	if len(requests) == 0 {
		t.Fatal("expected monitoring server to receive at least one query")
	}

	if err := cmd.Wait(); err != nil {
		t.Fatalf("shaper exited with error: %v\n%s", err, output.String())
	}
}

type healthPredicate func(status.Snapshot) bool

func waitForHealthSnapshot(ctx context.Context, url string, predicate healthPredicate) (status.Snapshot, error) {
	client := http.Client{ //nolint:exhaustruct // only timeout customised
		Timeout: time.Second,
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return status.Snapshot{}, fmt.Errorf("wait for health snapshot: %w", ctx.Err())
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
			if err != nil {
				return status.Snapshot{}, fmt.Errorf("build health request: %w", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				continue
			}

			body, readErr := io.ReadAll(resp.Body)
			closeErr := resp.Body.Close()
			if readErr != nil {
				return status.Snapshot{}, fmt.Errorf("read health response: %w", readErr)
			}

			if closeErr != nil {
				return status.Snapshot{}, fmt.Errorf("close health response: %w", closeErr)
			}

			if resp.StatusCode != http.StatusOK {
				continue
			}

			var snapshot status.Snapshot
			if err := json.Unmarshal(body, &snapshot); err != nil {
				return status.Snapshot{}, fmt.Errorf("decode health snapshot: %w", err)
			}

			if predicate(snapshot) {
				return snapshot, nil
			}
		}
	}
}
