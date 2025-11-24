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

func TestNewLoggerCLIInputs(t *testing.T) {
	t.Parallel()

	t.Run("InvalidLevelReturnsWrappedError", func(t *testing.T) {
		t.Parallel()

		logger, err := newLogger("cli-invalid")
		if logger != nil {
			t.Fatalf("expected nil logger for invalid level, got %v", logger)
		}

		if !errors.Is(err, errInvalidLogLevel) {
			t.Fatalf("expected invalid log level error, got %v", err)
		}
	})

	t.Run("EmptyLevelUsesDefault", func(t *testing.T) {
		t.Parallel()

		logger, err := newLogger("")
		if err != nil {
			t.Fatalf("unexpected error when using default level: %v", err)
		}

		defer func() {
			_ = logger.Sync()
		}()

		if !logger.Core().Enabled(zap.InfoLevel) {
			t.Fatalf("expected logger to enable %s level", defaultLogLevel)
		}

		if logger.Core().Enabled(zap.DebugLevel) {
			t.Fatalf(
				"expected logger to keep %s level below %s disabled",
				zap.DebugLevel,
				defaultLogLevel,
			)
		}
	})
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
