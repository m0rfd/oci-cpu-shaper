package main

import (
	"errors"
	"testing"

	"go.uber.org/zap"
)

func TestNewLoggerRejectsInvalidLevel(t *testing.T) {
	t.Parallel()

	_, err := newLogger("not-a-level")
	if err == nil {
		t.Fatal("expected error when creating logger with invalid level")
	}
}

func TestNewLoggerWrapsInvalidLevelError(t *testing.T) {
	t.Parallel()

	_, err := newLogger("boom")
	if !errors.Is(err, errInvalidLogLevel) {
		t.Fatalf("expected invalid log level error, got %v", err)
	}
}

func TestNewLoggerAppliesLevel(t *testing.T) {
	t.Parallel()

	logger, err := newLogger("debug")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() {
		_ = logger.Sync()
	}()

	if !logger.Core().Enabled(zap.DebugLevel) {
		t.Fatal("expected logger to enable debug level")
	}
}
