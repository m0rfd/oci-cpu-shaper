package e2eclient

import (
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/adapt"
)

type LoggingRecorder struct {
	logger   *zap.Logger
	delegate adapt.MetricsRecorder

	mu        sync.Mutex
	lastState string
}

// NewLoggingRecorder decorates the provided MetricsRecorder so e2e tests can observe
// controller state transitions via structured logs.
//

func NewLoggingRecorder(
	logger *zap.Logger,
	delegate adapt.MetricsRecorder,
) *LoggingRecorder {
	if logger == nil || delegate == nil {
		return nil
	}

	return &LoggingRecorder{ //nolint:exhaustruct // zero-value fields are intentional
		logger:   logger,
		delegate: delegate,
	}
}

func (r *LoggingRecorder) SetMode(mode string) {
	if r.delegate != nil {
		r.delegate.SetMode(mode)
	}
}

func (r *LoggingRecorder) SetState(state string) {
	trimmed := strings.TrimSpace(state)
	if r.delegate != nil {
		r.delegate.SetState(trimmed)
	}

	r.mu.Lock()

	previous := r.lastState
	if trimmed != previous {
		r.logger.Info(
			"controller state transition",
			zap.String("from", previous),
			zap.String("to", trimmed),
		)
		r.lastState = trimmed
	}

	r.mu.Unlock()
}

func (r *LoggingRecorder) SetTarget(target float64) {
	if r.delegate != nil {
		r.delegate.SetTarget(target)
	}
}

func (r *LoggingRecorder) ObserveOCIP95(value float64, fetchedAt time.Time) {
	if r.delegate != nil {
		r.delegate.ObserveOCIP95(value, fetchedAt)
	}
}

func (r *LoggingRecorder) ObserveHostCPU(utilisation float64) {
	if r.delegate != nil {
		r.delegate.ObserveHostCPU(utilisation)
	}
}

func (r *LoggingRecorder) SetInterval(interval time.Duration) {
	if r.delegate != nil {
		r.delegate.SetInterval(interval)
	}
}

func (r *LoggingRecorder) SetLastError(err error) {
	if r.delegate != nil {
		r.delegate.SetLastError(err)
	}
}
