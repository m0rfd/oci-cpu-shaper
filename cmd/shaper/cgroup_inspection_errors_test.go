package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/cgroup"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
)

var (
	errCgroupWeightBoom = errors.New("read cpu.weight: boom")
	errCgroupMaxBoom    = errors.New("read cpu.max: boom")
	errCgroupDetect     = errors.New("test detect error")
)

func TestDetectAndReportCgroupHandlesErrors(t *testing.T) {
	t.Parallel()

	exporter := metricshttp.NewExporter()
	logger, observed := newObservedLogger(zap.WarnLevel)

	info := detectAndReportCgroup(
		runDeps{ //nolint:exhaustruct // tests override detectCgroup only
			detectCgroup: func() (*cgroup.CPU, error) { return nil, errStubControllerRun },
		},
		logger,
		exporter,
	)
	if info != nil {
		t.Fatalf("expected nil cgroup info on error, got %+v", info)
	}

	body := renderExporter(t, exporter)
	for _, snippet := range []string{
		"cgroup_cpu_weight 0",
		"cgroup_cpu_max_quota 0",
	} {
		assertMetricContains(t, body, snippet)
	}

	warnEntries := observed.FilterMessage("failed to inspect cgroup cpu settings").All()
	if len(warnEntries) == 0 {
		t.Fatalf("expected warning log, got %#v", observed.All())
	}
}

func TestDetectAndReportCgroupUsesDefaultReaderFallback(t *testing.T) {
	t.Parallel()

	exporter := metricshttp.NewExporter()
	logger, observed := newObservedLogger(zap.WarnLevel)

	info := detectAndReportCgroup(
		runDeps{ //nolint:exhaustruct // tests override detectCgroup only
			detectCgroup: func() (*cgroup.CPU, error) { return nil, errCgroupDetect },
		},
		logger,
		exporter,
	)
	if info != nil {
		t.Fatalf("expected nil cgroup info on detection error, got %+v", info)
	}

	body := renderExporter(t, exporter)
	assertMetricContains(t, body, "cgroup_cpu_weight 0")
	assertMetricContains(t, body, "cgroup_cpu_max_quota 0")

	entries := observed.FilterMessage("failed to inspect cgroup cpu settings").All()
	if len(entries) == 0 {
		t.Fatalf("expected warning about cgroup detection failure, logs: %#v", observed.All())
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

func TestDetectCgroupInfoUsesDefaultReaderOnError(t *testing.T) {
	t.Parallel()

	cgroupReaderMu.Lock()

	previousReader := newCgroupReader

	cgroupReaderMu.Unlock()

	t.Cleanup(func() {
		cgroupReaderMu.Lock()

		newCgroupReader = previousReader

		cgroupReaderMu.Unlock()
	})

	tmp := t.TempDir()
	missingProc := filepath.Join(tmp, "missing.proc")

	cgroupReaderMu.Lock()

	newCgroupReader = func() cgroup.Reader {
		return cgroup.Reader{ProcPath: missingProc, RootPath: tmp}
	}

	cgroupReaderMu.Unlock()

	_, err := detectCgroupInfo(runDeps{}) //nolint:exhaustruct // fallback path uses default reader
	if err == nil || !strings.Contains(err.Error(), "detect cgroup") {
		t.Fatalf("expected detect cgroup error, got %v", err)
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
