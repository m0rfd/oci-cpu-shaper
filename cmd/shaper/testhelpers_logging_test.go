package main

import (
	"math"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const maxUint32 = ^uint32(0)

func fieldString(fields []zap.Field, key string) string {
	for _, field := range fields {
		if field.Key == key {
			return field.String
		}
	}

	return ""
}

func fieldBool(fields []zap.Field, key string) (bool, bool) {
	for _, field := range fields {
		if field.Key != key {
			continue
		}

		if field.Type == zapcore.BoolType {
			return field.Integer != 0, true
		}

		return false, true
	}

	return false, false
}

func fieldFloat(fields []zap.Field, key string) float64 {
	for _, field := range fields {
		if field.Key != key {
			continue
		}

		if field.Type == zapcore.Float64Type {
			if field.Integer < 0 {
				return 0
			}

			return math.Float64frombits(uint64(field.Integer))
		}

		if field.Type == zapcore.Float32Type {
			if field.Integer < 0 || field.Integer > int64(maxUint32) {
				return 0
			}

			return float64(math.Float32frombits(uint32(field.Integer)))
		}
	}

	return 0
}

func fieldInt(fields []zap.Field, key string) (int64, bool) {
	for _, field := range fields {
		if field.Key != key {
			continue
		}

		switch field.Type { //nolint:exhaustive // integer types only
		case zapcore.Int8Type,
			zapcore.Int16Type,
			zapcore.Int32Type,
			zapcore.Int64Type:
			return field.Integer, true
		case zapcore.Uint8Type,
			zapcore.Uint16Type,
			zapcore.Uint32Type,
			zapcore.Uint64Type:
			return field.Integer, true
		default:
			return 0, false
		}
	}

	return 0, false
}

func fieldDuration(fields []zap.Field, key string) (time.Duration, bool) {
	for _, field := range fields {
		if field.Key != key {
			continue
		}

		if field.Type == zapcore.DurationType {
			return time.Duration(field.Integer), true
		}

		return 0, true
	}

	return 0, false
}

func requireLogFieldString(t *testing.T, entry observer.LoggedEntry, key, want string) {
	t.Helper()

	if got := fieldString(entry.Context, key); got != want {
		t.Fatalf("expected %s field %q, got %+v", key, want, entry.Context)
	}
}

func requireLogFieldFloat(t *testing.T, entry observer.LoggedEntry, key string, want float64) {
	t.Helper()

	if got := fieldFloat(entry.Context, key); got != want {
		t.Fatalf("expected %s field %v, got %+v", key, want, entry.Context)
	}
}

func requireSingleEntry(
	t *testing.T,
	observed *observer.ObservedLogs,
	level zapcore.Level,
) observer.LoggedEntry {
	t.Helper()

	entries := observed.FilterLevelExact(level).All()
	if len(entries) == 0 {
		t.Fatalf("expected %s log entry, got %+v", level, observed.All())
	}

	return entries[0]
}
