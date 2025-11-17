package main

import (
	"math"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/adapt"
)

const (
	controllerTargetLogThreshold   = 0.005
	controllerObservationThreshold = 0.01
	controllerHostObservationDelta = 0.05
	controllerObservationCooldown  = 30 * time.Second
	hostPercentMultiplier          = 100.0
)

type controllerRecorderLogger struct {
	logger   *zap.Logger
	delegate adapt.MetricsRecorder

	mu sync.Mutex

	lastMode  string
	lastState string

	targetLogged bool
	lastTarget   float64

	ociLogged  bool
	lastOCI    float64
	lastOCILog time.Time

	hostLogged  bool
	lastHost    float64
	lastHostLog time.Time

	now func() time.Time
}

//nolint:ireturn // helper intentionally returns interface for wiring flexibility
func newRecorderLogger(
	logger *zap.Logger,
	delegate adapt.MetricsRecorder,
) adapt.MetricsRecorder {
	if logger == nil || delegate == nil {
		return delegate
	}

	return &controllerRecorderLogger{ //nolint:exhaustruct // zero-value fields capture prior state lazily
		logger:   logger,
		delegate: delegate,
		now:      time.Now,
	}
}

func (r *controllerRecorderLogger) SetMode(mode string) {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" {
		trimmed = "unknown"
	}

	if r.delegate != nil {
		r.delegate.SetMode(trimmed)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if trimmed == r.lastMode {
		return
	}

	r.logger.Info("controller mode configured", zap.String("mode", trimmed))
	r.lastMode = trimmed
}

func (r *controllerRecorderLogger) SetState(state string) {
	trimmed := strings.TrimSpace(state)
	if trimmed == "" {
		trimmed = "unknown"
	}

	if r.delegate != nil {
		r.delegate.SetState(trimmed)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if trimmed == r.lastState {
		return
	}

	r.logger.Info(
		"controller state transition",
		zap.String("from", r.lastState),
		zap.String("to", trimmed),
	)
	r.lastState = trimmed
}

func (r *controllerRecorderLogger) SetTarget(target float64) {
	if r.delegate != nil {
		r.delegate.SetTarget(target)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.targetLogged && math.Abs(target-r.lastTarget) < controllerTargetLogThreshold {
		return
	}

	r.logger.Debug(
		"controller target updated",
		zap.Float64("target", target),
		zap.Float64("previous", r.lastTarget),
	)
	r.targetLogged = true
	r.lastTarget = target
}

func (r *controllerRecorderLogger) ObserveOCIP95(value float64, fetchedAt time.Time) {
	if r.delegate != nil {
		r.delegate.ObserveOCIP95(value, fetchedAt)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.shouldLogObservation(
		r.ociLogged,
		r.lastOCI,
		value,
		r.lastOCILog,
		controllerObservationThreshold,
	) {
		return
	}

	fields := []zap.Field{zap.Float64("p95", value)}
	if !fetchedAt.IsZero() {
		fields = append(fields, zap.Time("fetchedAt", fetchedAt))
	}

	r.logger.Debug("oci metrics observation", fields...)
	r.ociLogged = true
	r.lastOCI = value
	r.lastOCILog = r.now()
}

func (r *controllerRecorderLogger) ObserveHostCPU(utilisation float64) {
	if r.delegate != nil {
		r.delegate.ObserveHostCPU(utilisation)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.shouldLogObservation(
		r.hostLogged,
		r.lastHost,
		utilisation,
		r.lastHostLog,
		controllerHostObservationDelta,
	) {
		return
	}

	percent := utilisation * hostPercentMultiplier
	if percent < 0 {
		percent = 0
	}

	r.logger.Debug(
		"host cpu observation",
		zap.Float64("percent", percent),
		zap.Float64("ratio", utilisation),
	)
	r.hostLogged = true
	r.lastHost = utilisation
	r.lastHostLog = r.now()
}

func (r *controllerRecorderLogger) shouldLogObservation(
	logged bool,
	previous float64,
	next float64,
	lastLogged time.Time,
	threshold float64,
) bool {
	if !logged {
		return true
	}

	if math.Abs(next-previous) >= threshold {
		return true
	}

	if lastLogged.IsZero() {
		return true
	}

	now := r.now
	if now == nil {
		now = time.Now
	}

	if now().Sub(lastLogged) >= controllerObservationCooldown {
		return true
	}

	return false
}
