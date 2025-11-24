package main

import (
	"testing"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/cgroup"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
)

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

	logger, observed := newObservedLogger(zap.InfoLevel)
	detectAndReportCgroup(
		runDeps{ //nolint:exhaustruct // tests override detectCgroup only
			detectCgroup: func() (*cgroup.CPU, error) { return info, nil },
		},
		logger,
		metricshttp.NewExporter(),
	)

	warnEntries := observed.FilterMessage("cpu.weight exceeds recommended low-weight baseline").
		All()
	if len(warnEntries) == 0 {
		t.Fatalf("expected warning about high cpu.weight, logs: %#v", observed.All())
	}
}

func TestCgroupWeightFieldsWarnsOnHighWeight(t *testing.T) {
	t.Parallel()

	logger, observed := newObservedLogger(zap.WarnLevel)

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

	logger, observed := newObservedLogger(zap.InfoLevel)

	logCgroupInfo(logger, nil)

	if count := len(observed.All()); count != 0 {
		t.Fatalf("expected no logs when info is nil, got %d entries", count)
	}

	logCgroupInfo(logger, info)

	if entries := observed.FilterMessage("detected cgroup cpu settings").All(); len(entries) != 1 {
		t.Fatalf("expected log after info supplied, got %#v", observed.All())
	}
}
