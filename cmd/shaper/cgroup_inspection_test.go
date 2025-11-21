package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"oci-cpu-shaper/pkg/cgroup"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
)

var (
	errCgroupWeightBoom = errors.New("read cpu.weight: boom")
	errCgroupMaxBoom    = errors.New("read cpu.max: boom")
	errCgroupDetect     = errors.New("test detect error")
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
			Period:    100000,
			Unlimited: true,
			Available: true,
			Err:       nil,
		},
	}

	recordCgroupMetrics(exporter, info)

	body := renderExporter(t, exporter)
	assertMetricContains(t, body, "cgroup_cpu_weight 0")
	assertMetricContains(t, body, "cgroup_cpu_max_quota 0")
	assertMetricContains(t, body, "cgroup_cpu_max_period 100000")
	assertMetricContains(t, body, "cgroup_cpu_max_unlimited 1")
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

	logCgroupInfo(nil, info)

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)

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
		t.Fatalf("expected single field on weight error, got %d", len(fields))
	}

	errField := fields[0]
	if errField.Key != "cpuWeightError" {
		t.Fatalf("expected cpuWeightError key, got %s", errField.Key)
	}

	fields = cgroupWeightFields(
		nil,
		cgroup.Weight{Path: "", Value: 0, Available: false, Err: nil},
	)
	if len(fields) != 1 {
		t.Fatalf("expected single field on unavailable weight, got %d", len(fields))
	}

	statusField := fields[0]
	if statusField.Key != "cpuWeightStatus" {
		t.Fatalf("expected cpuWeightStatus key, got %s", statusField.Key)
	}
}

func TestCgroupWeightFieldsWarnsOnHighWeight(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	cgroupWeightFields(logger, cgroup.Weight{
		Path:      "",
		Value:     cgroupLowWeightBaseline + 1,
		Available: true,
		Err:       nil,
	})

	entries := observed.FilterMessage("cpu.weight exceeds recommended low-weight baseline").All()
	if len(entries) == 0 {
		t.Fatalf("expected warning when weight is above baseline, logs: %#v", observed.All())
	}
}

func TestCgroupWeightFieldsReturnsValue(t *testing.T) {
	t.Parallel()

	fields := cgroupWeightFields(nil, cgroup.Weight{
		Path:      "cpu.weight",
		Value:     1000,
		Available: true,
		Err:       nil,
	})

	if len(fields) != 1 {
		t.Fatalf("expected one weight field, got %d", len(fields))
	}

	if fields[0].Key != "cpuWeight" {
		t.Fatalf("expected cpuWeight field, got %s", fields[0].Key)
	}
}

func TestCgroupMaxFieldsHandlesErrorAndUnavailable(t *testing.T) {
	t.Parallel()

	fields := cgroupMaxFields(cgroup.Max{
		Path:      "",
		Quota:     0,
		Period:    0,
		Unlimited: false,
		Available: false,
		Err:       errCgroupMaxBoom,
	})

	if len(fields) != 1 {
		t.Fatalf("expected single field on max error, got %d", len(fields))
	}

	errField := fields[0]
	if errField.Key != "cpuMaxError" {
		t.Fatalf("expected cpuMaxError key, got %s", errField.Key)
	}

	fields = cgroupMaxFields(cgroup.Max{
		Path:      "",
		Quota:     0,
		Period:    0,
		Unlimited: false,
		Available: false,
		Err:       nil,
	})

	if len(fields) != 1 {
		t.Fatalf("expected single field on unavailable max, got %d", len(fields))
	}

	statusField := fields[0]
	if statusField.Key != "cpuMaxStatus" {
		t.Fatalf("expected cpuMaxStatus key, got %s", statusField.Key)
	}
}

func TestCgroupMaxFieldsReturnsValues(t *testing.T) {
	t.Parallel()

	fields := cgroupMaxFields(cgroup.Max{
		Path:      "cpu.max",
		Quota:     1000,
		Period:    100000,
		Unlimited: false,
		Available: true,
		Err:       nil,
	})

	if len(fields) != 3 {
		t.Fatalf("expected three fields for limited max, got %d", len(fields))
	}

	keys := map[string]struct{}{
		"cpuMaxQuota":     {},
		"cpuMaxPeriod":    {},
		"cpuMaxUnlimited": {},
	}

	for _, field := range fields {
		if _, ok := keys[field.Key]; !ok {
			t.Fatalf("unexpected field key %s", field.Key)
		}
	}

	fields = cgroupMaxFields(cgroup.Max{
		Path:      "cpu.max",
		Quota:     0,
		Period:    100000,
		Unlimited: true,
		Available: true,
		Err:       nil,
	})

	if len(fields) != 2 {
		t.Fatalf("expected two fields for unlimited max, got %d", len(fields))
	}

	if fields[0].Key != "cpuMaxUnlimited" {
		t.Fatalf("expected cpuMaxUnlimited field, got %s", fields[0].Key)
	}

	if fields[1].Key != "cpuMaxPeriod" {
		t.Fatalf("expected cpuMaxPeriod field, got %s", fields[1].Key)
	}
}

func TestCgroupMaxFieldsSupportsAddingFields(t *testing.T) {
	t.Parallel()

	fields := []zap.Field{zap.String("hello", "world")}

	got := append([]zap.Field{}, fields...)
	got = append(got, cgroupMaxFields(cgroup.Max{
		Path:      "cpu.max",
		Quota:     0,
		Period:    100000,
		Unlimited: true,
		Available: true,
		Err:       nil,
	})...)

	want := []zap.Field{
		zap.String("hello", "world"),
		zap.Bool("cpuMaxUnlimited", true),
		zap.Uint64("cpuMaxPeriod", 100000),
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
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
