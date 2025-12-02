package main

import (
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestNewLoggerInvalidLevelReturnsWrappedError(t *testing.T) {
	t.Parallel()

	logger, err := newLogger("not-supported")
	if logger != nil {
		t.Fatalf("expected nil logger for invalid level, got %v", logger)
	}

	if !errors.Is(err, errInvalidLogLevel) {
		t.Fatalf("expected invalid log level error, got %v", err)
	}
}

//nolint:paralleltest // serial to safely swap logger factory for failure coverage
func TestNewLoggerBuildFailure(t *testing.T) {
	originalProductionConfig := newProductionConfig

	newProductionConfigMu.Lock()

	newProductionConfig = func() zap.Config {
		cfg := zap.NewProductionConfig()
		cfg.OutputPaths = []string{"://invalid-path"}

		return cfg
	}

	newProductionConfigMu.Unlock()

	t.Cleanup(func() {
		newProductionConfigMu.Lock()

		newProductionConfig = originalProductionConfig

		newProductionConfigMu.Unlock()
	})

	logger, err := newLogger("info")
	if logger != nil {
		t.Fatalf("expected nil logger when build fails, got %v", logger)
	}

	if err == nil {
		t.Fatal("expected error when zap logger build fails")
	}

	if !strings.Contains(err.Error(), "build zap logger") {
		t.Fatalf("expected build zap logger error, got %v", err)
	}
}
