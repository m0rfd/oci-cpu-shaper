package main

import (
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/cgroup"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
)

const cgroupLowWeightBaseline = 128

//nolint:gochecknoglobals // test seam for default cgroup reader construction
var newCgroupReader = func() cgroup.Reader {
	return cgroup.Reader{ProcPath: "", RootPath: ""}
}

//nolint:gochecknoglobals // guarded global allows tests to swap cgroup reader factory
var cgroupReaderMu sync.RWMutex

func detectAndReportCgroup(
	deps runDeps,
	logger *zap.Logger,
	exporter *metricshttp.Exporter,
) *cgroup.CPU {
	info, err := detectCgroupInfo(deps)
	if err != nil {
		if logger != nil {
			logger.Warn("failed to inspect cgroup cpu settings", zap.Error(err))
		}

		recordCgroupMetrics(exporter, nil)

		return nil
	}

	recordCgroupMetrics(exporter, info)
	logCgroupInfo(logger, info)

	return info
}

func detectCgroupInfo(deps runDeps) (*cgroup.CPU, error) {
	if deps.detectCgroup != nil {
		return deps.detectCgroup()
	}

	cgroupReaderMu.RLock()

	readerFactory := newCgroupReader

	cgroupReaderMu.RUnlock()

	reader := readerFactory()

	info, err := reader.Detect()
	if err != nil {
		return nil, fmt.Errorf("detect cgroup: %w", err)
	}

	return info, nil
}

func recordCgroupMetrics(exporter *metricshttp.Exporter, info *cgroup.CPU) {
	if exporter == nil {
		return
	}

	weight := uint64(0)
	if info != nil && info.Weight.Err == nil && info.Weight.Available {
		weight = info.Weight.Value
	}

	exporter.SetCgroupCPUWeight(weight)

	var (
		quota  uint64
		period uint64
	)

	unlimited := false

	if info != nil && info.Max.Err == nil && info.Max.Available {
		period = info.Max.Period

		unlimited = info.Max.Unlimited
		if !info.Max.Unlimited {
			quota = info.Max.Quota
		}
	}

	exporter.SetCgroupCPUMax(quota, period, unlimited)
}

func logCgroupInfo(logger *zap.Logger, info *cgroup.CPU) {
	if logger == nil || info == nil {
		return
	}

	fields := []zap.Field{zap.String("path", strings.TrimSpace(info.Path))}
	fields = append(fields, cgroupWeightFields(logger, info.Weight)...)
	fields = append(fields, cgroupMaxFields(info.Max)...)

	logger.Info("detected cgroup cpu settings", fields...)
}

func cgroupWeightFields(logger *zap.Logger, weight cgroup.Weight) []zap.Field {
	switch {
	case weight.Err != nil:
		return []zap.Field{zap.String("cpuWeightError", weight.Err.Error())}
	case weight.Available:
		field := zap.Uint64("cpuWeight", weight.Value)
		if logger != nil && weight.Value > cgroupLowWeightBaseline {
			logger.Warn(
				"cpu.weight exceeds recommended low-weight baseline",
				zap.Uint64("weight", weight.Value),
				zap.Uint64("baseline", cgroupLowWeightBaseline),
			)
		}

		return []zap.Field{field}
	default:
		return []zap.Field{zap.String("cpuWeightStatus", "unavailable")}
	}
}

func cgroupMaxFields(cpuMax cgroup.Max) []zap.Field {
	switch {
	case cpuMax.Err != nil:
		return []zap.Field{zap.String("cpuMaxError", cpuMax.Err.Error())}
	case cpuMax.Available:
		fields := []zap.Field{
			zap.Bool("cpuMaxUnlimited", cpuMax.Unlimited),
			zap.Uint64("cpuMaxPeriod", cpuMax.Period),
		}
		if !cpuMax.Unlimited {
			fields = append(fields, zap.Uint64("cpuMaxQuota", cpuMax.Quota))
		}

		return fields
	default:
		return []zap.Field{zap.String("cpuMaxStatus", "unavailable")}
	}
}
