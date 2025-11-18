package main

import (
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"oci-cpu-shaper/pkg/cgroup"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
)

func TestDetectAndReportCgroupPublishesMetrics(t *testing.T) {
	t.Parallel()

	info := &cgroup.CPU{
		Path: "/user.slice/shaper.scope",
		Weight: cgroup.Weight{
			Path:      "",
			Value:     cgroupLowWeightBaseline,
			Available: true,
			Err:       nil,
		},
		Max: cgroup.Max{
			Path:      "",
			Quota:     60000,
			Period:    100000,
			Unlimited: false,
			Available: true,
			Err:       nil,
		},
	}

	exporter := metricshttp.NewExporter()
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	var deps runDeps

	deps.detectCgroup = func() (*cgroup.CPU, error) {
		return info, nil
	}

	got := detectAndReportCgroup(deps, logger, exporter)
	if got != info {
		t.Fatalf("expected cgroup info to be returned, got %+v", got)
	}

	metrics, err := exporter.Render()
	if err != nil {
		t.Fatalf("render metrics: %v", err)
	}

	body := string(metrics)
	for _, snippet := range []string{
		"cgroup_cpu_weight 128",
		"cgroup_cpu_max_quota 60000",
		"cgroup_cpu_max_period 100000",
	} {
		if !strings.Contains(body, snippet) {
			t.Fatalf("expected metrics output to contain %q, got:\n%s", snippet, body)
		}
	}

	entries := observed.FilterMessage("detected cgroup cpu settings").All()
	if len(entries) == 0 {
		logOutput := observed.All()
		t.Fatalf("expected cgroup detection log, got %#v", logOutput)
	}
}

func TestDetectAndReportCgroupWarnsOnHighWeight(t *testing.T) {
	t.Parallel()

	info := &cgroup.CPU{
		Path: "/slice",
		Weight: cgroup.Weight{
			Path:      "",
			Value:     cgroupLowWeightBaseline + 10,
			Available: true,
			Err:       nil,
		},
		Max: cgroup.Max{
			Path:      "",
			Quota:     0,
			Period:    100000,
			Unlimited: true,
			Available: true,
			Err:       nil,
		},
	}

	var deps runDeps

	deps.detectCgroup = func() (*cgroup.CPU, error) {
		return info, nil
	}
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	detectAndReportCgroup(deps, logger, metricshttp.NewExporter())

	warnEntries := observed.FilterMessage("cpu.weight exceeds recommended low-weight baseline").
		All()
	if len(warnEntries) == 0 {
		entries := observed.All()
		t.Fatalf("expected warning about high cpu.weight, logs: %#v", entries)
	}
}

func TestDetectAndReportCgroupHandlesErrors(t *testing.T) {
	t.Parallel()

	var deps runDeps

	deps.detectCgroup = func() (*cgroup.CPU, error) {
		return nil, errStubControllerRun
	}
	exporter := metricshttp.NewExporter()
	core, observed := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	info := detectAndReportCgroup(deps, logger, exporter)
	if info != nil {
		t.Fatalf("expected nil cgroup info on error, got %+v", info)
	}

	metrics, err := exporter.Render()
	if err != nil {
		t.Fatalf("render metrics: %v", err)
	}

	body := string(metrics)
	for _, snippet := range []string{
		"cgroup_cpu_weight 0",
		"cgroup_cpu_max_quota 0",
	} {
		if !strings.Contains(body, snippet) {
			t.Fatalf("expected metric %q to be present, output:\n%s", snippet, body)
		}
	}

	warnEntries := observed.FilterMessage("failed to inspect cgroup cpu settings").All()
	if len(warnEntries) == 0 {
		logs := observed.All()
		t.Fatalf("expected warning log, got %#v", logs)
	}
}

func TestLogCgroupInfoSkipsWithoutLoggerOrInfo(t *testing.T) {
	t.Parallel()

	info := &cgroup.CPU{
		Path:   "/slice",
		Weight: cgroup.Weight{Path: "", Value: 0, Available: false, Err: nil},
		Max: cgroup.Max{
			Path:      "",
			Quota:     0,
			Period:    0,
			Unlimited: false,
			Available: false,
			Err:       nil,
		},
	}

	// nil logger should be tolerated so metrics-only deployments can reuse the helper.
	logCgroupInfo(nil, info)

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	// nil info indicates detection failed; no log entries should be emitted.
	logCgroupInfo(logger, nil)

	if count := len(observed.All()); count != 0 {
		t.Fatalf("expected no logs when info is nil, got %d entries", count)
	}

	logCgroupInfo(logger, info)

	if entries := observed.FilterMessage("detected cgroup cpu settings").All(); len(entries) != 1 {
		logs := observed.All()
		t.Fatalf("expected log after info supplied, got %#v", logs)
	}
}

func TestCgroupWeightFieldsHandlesErrorAndUnavailable(t *testing.T) {
	t.Parallel()

	fields := cgroupWeightFields(
		nil,
		cgroup.Weight{Path: "", Value: 0, Available: false, Err: errCgroupWeightBoom},
	)
	if len(fields) != 1 {
		t.Fatalf("expected single error field, got %d", len(fields))
	}

	field := fields[0]
	if field.Key != "cpuWeightError" || field.Type != zapcore.StringType {
		t.Fatalf("unexpected error field: %#v", field)
	}

	if field.String != errCgroupWeightBoom.Error() {
		t.Fatalf(
			"expected error field to capture %q, got %q",
			errCgroupWeightBoom.Error(),
			field.String,
		)
	}

	unavailable := cgroupWeightFields(
		nil,
		cgroup.Weight{Path: "", Value: 0, Available: false, Err: nil},
	)
	if len(unavailable) != 1 {
		t.Fatalf("expected unavailable status, got %d fields", len(unavailable))
	}

	status := unavailable[0]
	if status.Key != "cpuWeightStatus" || status.String != "unavailable" {
		t.Fatalf("unexpected status field: %#v", status)
	}
}

func TestCgroupMaxFieldsHandlesErrorAndUnavailable(t *testing.T) {
	t.Parallel()

	fields := cgroupMaxFields(
		cgroup.Max{
			Path:      "",
			Quota:     0,
			Period:    0,
			Unlimited: false,
			Available: false,
			Err:       errCgroupMaxBoom,
		},
	)
	if len(fields) != 1 {
		t.Fatalf("expected single error field, got %d", len(fields))
	}

	field := fields[0]
	if field.Key != "cpuMaxError" || field.String != errCgroupMaxBoom.Error() {
		t.Fatalf("unexpected error field: %#v", field)
	}

	unavailable := cgroupMaxFields(
		cgroup.Max{
			Path:      "",
			Quota:     0,
			Period:    0,
			Unlimited: false,
			Available: false,
			Err:       nil,
		},
	)
	if len(unavailable) != 1 {
		t.Fatalf("expected unavailable field, got %d", len(unavailable))
	}

	status := unavailable[0]
	if status.Key != "cpuMaxStatus" || status.String != "unavailable" {
		t.Fatalf("unexpected status field: %#v", status)
	}
}
