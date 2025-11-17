//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	interne2e "oci-cpu-shaper/tests/internal/e2e"
)

func TestCLISignalShutdown(t *testing.T) {
	repoRoot := interne2e.RepositoryRoot(t)
	binary := interne2e.BuildShaperBinary(t, repoRoot, "e2e")

	signals := []struct {
		name   string
		signal os.Signal
	}{
		{name: "SIGTERM", signal: syscall.SIGTERM},
		{name: "SIGINT", signal: os.Interrupt},
	}

	for _, tc := range signals {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runSignalShutdownScenario(t, binary, tc.name, tc.signal)
		})
	}
}

func runSignalShutdownScenario(t *testing.T, binary, name string, sig os.Signal) {
	t.Helper()

	scenarioCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	metricsPort := interne2e.FreePort(t)
	configPath := writeConfig(t, fmt.Sprintf("signal-%s.yaml", strings.ToLower(name)), fmt.Sprintf(`
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
  offline: true
`, metricsPort))

	var output bytes.Buffer

	cmd := exec.CommandContext(scenarioCtx, binary, "--config", configPath, "--log-level", "debug")
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Start(); err != nil {
		t.Fatalf("start shaper: %v", err)
	}

	metricsURL := fmt.Sprintf("http://127.0.0.1:%d/metrics", metricsPort)
	if _, err := interne2e.WaitForMetrics(scenarioCtx, metricsURL); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("wait for metrics: %v\nlogs:\n%s", err, output.String())
	}

	if err := cmd.Process.Signal(sig); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("send %s: %v", name, err)
	}

	if err := cmd.Wait(); err != nil {
		t.Fatalf("shaper exited with error after %s: %v\nlogs:\n%s", name, err, output.String())
	}

	logs := parseLogEntries(t, output.Bytes())
	assertLogField(t, logs, "controller stopped", "reason", context.Canceled.Error())
	assertLogField(t, logs, "stopping metrics server", "bind", fmt.Sprintf("127.0.0.1:%d", metricsPort))
}

func assertLogField(t *testing.T, logs []logEntry, message, field, expected string) {
	t.Helper()

	for _, entry := range logs {
		msg, _ := entry["message"].(string)
		if msg != message {
			continue
		}

		value, _ := entry[field].(string)
		if value != expected {
			t.Fatalf("expected %s log %s=%q, got %q", message, field, expected, value)
		}

		return
	}

	t.Fatalf("expected log message %q not found", message)
}
