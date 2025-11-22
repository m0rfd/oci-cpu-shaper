//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"
	"time"

	interne2e "oci-cpu-shaper/tests/internal/e2e"
)

func TestMetricsDisabledWhenBindIsWhitespace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	repoRoot := interne2e.RepositoryRoot(t)
	binary := interne2e.BuildShaperBinary(t, repoRoot, "e2e")

	metricsPort := interne2e.FreePort(t)
	configPath := writeConfig(t, "metrics-disabled.yaml", `controller:
  interval: 1s
  relaxedInterval: 2s
estimator:
  interval: 200ms
pool:
  workers: 1
  quantum: 150ms
http:
  bind: "    "
oci:
  offline: true
`)

	logs, _ := runShaperWithOptions(ctx, t, binary, configPath, shaperRunOptions{
		metricsPort:    metricsPort,
		env:            map[string]string{},
		waitForMetrics: false,
		onStart: func() {
			assertMetricsPortUnused(t, metricsPort, 3*time.Second)
		},
	})

	assertMetricsPortUnused(t, metricsPort, time.Second)
	requireMetricsDisabledReason(t, logs, "http bind address empty")
}

func assertMetricsPortUnused(t *testing.T, port int, duration time.Duration) {
	t.Helper()

	deadline := time.Now().Add(duration)
	address := fmt.Sprintf("127.0.0.1:%d", port)

	for {
		conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			t.Fatalf("expected metrics port %d to reject connections", port)
		}

		if !errors.Is(err, syscall.ECONNREFUSED) {
			t.Fatalf("expected connection refused on metrics port %d, got %v", port, err)
		}

		if time.Now().After(deadline) {
			return
		}

		time.Sleep(50 * time.Millisecond)
	}
}

func requireMetricsDisabledReason(t *testing.T, logs []logEntry, reason string) {
	t.Helper()

	for _, entry := range logs {
		message, _ := entry["message"].(string)
		if message != "metrics server disabled" {
			continue
		}

		observedReason, _ := entry["reason"].(string)
		if observedReason == reason {
			return
		}
	}

	t.Fatalf("expected metrics server disabled log with reason %q", reason)
}
