package main

import (
	"reflect"
	"testing"

	"go.uber.org/zap"
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
	logger, observed := newObservedLogger(zap.InfoLevel)

	got := detectAndReportCgroup(
		runDeps{ //nolint:exhaustruct // tests override detectCgroup only
			detectCgroup: func() (*cgroup.CPU, error) { return info, nil },
		},
		logger,
		exporter,
	)
	if got != info {
		t.Fatalf("expected cgroup info to be returned, got %+v", got)
	}

	metrics := renderExporter(t, exporter)
	for _, snippet := range []string{
		"cgroup_cpu_weight 128",
		"cgroup_cpu_max_quota 60000",
		"cgroup_cpu_max_period 100000",
	} {
		assertMetricContains(t, metrics, snippet)
	}

	entries := observed.FilterMessage("detected cgroup cpu settings").All()
	if len(entries) == 0 {
		t.Fatalf("expected cgroup detection log, got %#v", observed.All())
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
