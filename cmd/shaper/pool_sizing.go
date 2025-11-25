package main

import (
	"context"
	"fmt"
	"math"
	"strings"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

const (
	minAutoSizedWorkers = 1
	maxAutoSizedWorkers = 32
)

type poolSizingResult struct {
	applied    bool
	capped     bool
	workers    int
	shapeOCPUs float64
}

func applyPoolSizingFromShape(
	ctx context.Context,
	cfg runtimeconfig.Config,
	mode string,
	imdsClient imds.Client,
) (runtimeconfig.Config, poolSizingResult, error) {
	empty := poolSizingResult{
		applied:    false,
		capped:     false,
		workers:    cfg.Pool.Workers,
		shapeOCPUs: 0,
	}

	if !cfg.Pool.AutoSizeFromShape {
		return cfg, empty, nil
	}

	if strings.TrimSpace(mode) == modeNoop || cfg.OCI.Offline {
		return cfg, empty, nil
	}

	if imdsClient == nil {
		return cfg, empty, errControllerIMDSRequired
	}

	shapeCfg, err := imdsClient.ShapeConfig(ctx)
	if err != nil {
		return cfg, empty, fmt.Errorf("lookup shape config: %w", err)
	}

	workers, applied, capped := deriveWorkerCountFromOCPUs(shapeCfg.OCPUs, cfg.Pool.Workers)
	if applied {
		cfg.Pool.Workers = workers
	}

	return cfg, poolSizingResult{
		applied:    applied,
		capped:     capped,
		workers:    workers,
		shapeOCPUs: shapeCfg.OCPUs,
	}, nil
}

func deriveWorkerCountFromOCPUs(ocpus float64, fallback int) (int, bool, bool) {
	if ocpus <= 0 {
		return fallback, false, false
	}

	workers := int(math.Ceil(ocpus))
	capped := false

	if workers < minAutoSizedWorkers {
		workers = minAutoSizedWorkers
		capped = true
	}

	if workers > maxAutoSizedWorkers {
		workers = maxAutoSizedWorkers
		capped = true
	}

	return workers, true, capped
}

func logPoolSizing(logger *zap.Logger, result poolSizingResult) {
	if logger == nil || !result.applied {
		return
	}

	fields := []zap.Field{
		zap.Float64("shapeOCPUs", result.shapeOCPUs),
		zap.Int("workerCount", result.workers),
		zap.Int("workerCapMin", minAutoSizedWorkers),
		zap.Int("workerCapMax", maxAutoSizedWorkers),
	}

	if result.capped {
		fields = append(fields, zap.Bool("capped", true))
	}

	logger.Info("sized worker pool from shape config", fields...)
}
