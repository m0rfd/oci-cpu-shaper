//go:build integration

package integration

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"oci-cpu-shaper/internal/e2eclient"
	"oci-cpu-shaper/pkg/adapt"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
	"oci-cpu-shaper/pkg/oci"
)

func TestStaticMetricsClientFeedsControllerMetrics(t *testing.T) {
	logger := zap.NewNop()
	exporter := metricshttp.NewExporter()
	recorder := e2eclient.NewLoggingRecorder(logger, exporter)

	cfg := adapt.DefaultConfig()
	cfg.ResourceID = "ocid1.instance.oc1..integration-static"
	cfg.Interval = 100 * time.Millisecond
	cfg.RelaxedInterval = 100 * time.Millisecond

	const staticP95 = 0.37

	shaper := newRecordingShaper()

	controller, err := adapt.NewAdaptiveController(cfg, oci.NewStaticMetricsClient(staticP95), nil, shaper, recorder)
	if err != nil {
		t.Fatalf("create adaptive controller: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- controller.Run(ctx)
	}()

	waitForLastP95(t, controller, staticP95, ctx)

	metrics := waitForMetricsSnapshot(t, exporter, staticP95, ctx)

	assertLastSuccessEpoch(t, metrics)

	cancel()

	if runErr := <-errCh; runErr != nil && !errors.Is(runErr, context.Canceled) {
		t.Fatalf("controller run returned error: %v", runErr)
	}
}

func waitForLastP95(t *testing.T, controller *adapt.AdaptiveController, expected float64, ctx context.Context) {
	t.Helper()

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for LastP95 to reach %.2f", expected)
		case <-ticker.C:
			if math.Abs(controller.LastP95()-expected) < 1e-9 {
				return
			}
		}
	}
}

func waitForMetricsSnapshot(t *testing.T, exporter *metricshttp.Exporter, expected float64, ctx context.Context) []byte {
	t.Helper()

	targetLine := fmt.Sprintf("oci_p95 %.6f", expected)

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for metrics to contain %q", targetLine)
		case <-ticker.C:
			snapshot, err := exporter.Render()
			if err != nil {
				t.Fatalf("render metrics: %v", err)
			}

			if bytes.Contains(snapshot, []byte(targetLine)) {
				return snapshot
			}
		}
	}
}

func assertLastSuccessEpoch(t *testing.T, metrics []byte) {
	t.Helper()

	scanner := bufio.NewScanner(bytes.NewReader(metrics))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "oci_last_success_epoch ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("unexpected last success epoch format: %q", line)
		}

		epoch, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			t.Fatalf("parse epoch from %q: %v", line, err)
		}

		if epoch == 0 {
			t.Fatalf("expected last success epoch to be recorded, got %q", line)
		}

		return
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scan metrics: %v", err)
	}

	t.Fatalf("oci_last_success_epoch not found in metrics:\n%s", metrics)
}
