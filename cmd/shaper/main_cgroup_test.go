package main

import (
	"errors"
	"strings"
	"testing"

	"oci-cpu-shaper/pkg/cgroup"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
)

var errCgroupDetect = errors.New("test detect error")

func TestDetectCgroupInfoUsesInjectedDetector(t *testing.T) {
	t.Parallel()

	expected := &cgroup.CPU{
		Path: "",
		Weight: cgroup.Weight{
			Path:      "",
			Value:     0,
			Available: false,
			Err:       nil,
		},
		Max: cgroup.Max{
			Path:      "",
			Quota:     0,
			Period:    0,
			Unlimited: false,
			Available: false,
			Err:       nil,
		},
	}
	deps := runDeps{ //nolint:exhaustruct // tests override detectCgroup only
		detectCgroup: func() (*cgroup.CPU, error) {
			return expected, nil
		},
	}

	info, err := detectCgroupInfo(deps)
	if err != nil {
		t.Fatalf("detectCgroupInfo returned error: %v", err)
	}

	if info != expected {
		t.Fatalf("expected injected info, got %#v", info)
	}
}

func TestDetectCgroupInfoDefaultPath(t *testing.T) {
	t.Parallel()

	info, err := detectCgroupInfo(
		runDeps{}, //nolint:exhaustruct // default zero-value deps exercise reader path
	)
	if err != nil {
		t.Skipf("cgroup detection unavailable: %v", err)
	}

	if info == nil {
		t.Fatal("expected cgroup detection to return info")
	}
}

func TestDetectCgroupInfoPropagatesErrors(t *testing.T) {
	t.Parallel()

	deps := runDeps{ //nolint:exhaustruct // tests override detectCgroup only
		detectCgroup: func() (*cgroup.CPU, error) {
			return nil, errCgroupDetect
		},
	}

	_, err := detectCgroupInfo(deps)
	if !errors.Is(err, errCgroupDetect) {
		t.Fatalf("expected %v, got %v", errCgroupDetect, err)
	}
}

func TestRecordCgroupMetricsHandlesNilExporter(t *testing.T) {
	t.Parallel()

	recordCgroupMetrics(nil, nil)
}

func TestRecordCgroupMetricsWritesValues(t *testing.T) {
	t.Parallel()

	exporter := metricshttp.NewExporter()
	info := &cgroup.CPU{
		Path:   "/sys/fs/cgroup/test",
		Weight: cgroup.Weight{Path: "cpu.weight", Value: 2000, Available: true, Err: nil},
		Max: cgroup.Max{
			Path:      "cpu.max",
			Quota:     50000,
			Period:    100000,
			Unlimited: false,
			Available: true,
			Err:       nil,
		},
	}

	recordCgroupMetrics(exporter, info)

	body := renderExporter(t, exporter)
	assertMetricContains(t, body, "cgroup_cpu_weight 2000")
	assertMetricContains(t, body, "cgroup_cpu_max_quota 50000")
	assertMetricContains(t, body, "cgroup_cpu_max_period 100000")
	assertMetricContains(t, body, "cgroup_cpu_max_unlimited 0")
}

func TestRecordCgroupMetricsMarksUnlimited(t *testing.T) {
	t.Parallel()

	exporter := metricshttp.NewExporter()
	info := &cgroup.CPU{
		Path:   "/sys/fs/cgroup/test",
		Weight: cgroup.Weight{Path: "cpu.weight", Value: 0, Available: false, Err: nil},
		Max: cgroup.Max{
			Path:      "cpu.max",
			Quota:     0,
			Period:    50000,
			Unlimited: true,
			Available: true,
			Err:       nil,
		},
	}

	recordCgroupMetrics(exporter, info)

	body := renderExporter(t, exporter)
	assertMetricContains(t, body, "cgroup_cpu_max_quota 0")
	assertMetricContains(t, body, "cgroup_cpu_max_period 50000")
	assertMetricContains(t, body, "cgroup_cpu_max_unlimited 1")
}

func renderExporter(t *testing.T, exporter *metricshttp.Exporter) string {
	t.Helper()

	data, err := exporter.Render()
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	return string(data)
}

func assertMetricContains(t *testing.T, metrics, want string) {
	t.Helper()

	if !strings.Contains(metrics, want) {
		t.Fatalf("expected metrics to contain %q, got %q", want, metrics)
	}
}
