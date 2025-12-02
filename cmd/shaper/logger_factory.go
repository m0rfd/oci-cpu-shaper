package main

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
)

var (
	//nolint:gochecknoglobals // swapped in tests to exercise zap build errors
	newProductionConfig = zap.NewProductionConfig
	//nolint:gochecknoglobals // guards test replacement of newProductionConfig
	newProductionConfigMu sync.RWMutex
)

func newLogger(level string) (*zap.Logger, error) {
	if level == "" {
		level = defaultLogLevel
	}

	newProductionConfigMu.RLock()

	cfg := newProductionConfig()

	newProductionConfigMu.RUnlock()

	err := cfg.Level.UnmarshalText([]byte(level))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidLogLevel, err)
	}

	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.MessageKey = "message"
	cfg.EncoderConfig.LevelKey = "level"
	cfg.EncoderConfig.CallerKey = "caller"

	logger, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("build zap logger: %w", err)
	}

	return logger, nil
}
