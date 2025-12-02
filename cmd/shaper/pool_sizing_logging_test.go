package main

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestLogPoolSizingSkipsWhenNotApplied(t *testing.T) {
	t.Parallel()

	logger, observed := observer.New(zapcore.InfoLevel)

	logPoolSizing(zap.New(logger), poolSizingResult{ //nolint:exhaustruct
		applied: false,
	})

	if entries := observed.All(); len(entries) != 0 {
		t.Fatalf("expected no log entries, got %+v", entries)
	}
}

func TestLogPoolSizingEmitsFields(t *testing.T) {
	t.Parallel()

	logger, observed := observer.New(zapcore.InfoLevel)

	logPoolSizing(zap.New(logger), poolSizingResult{ //nolint:exhaustruct
		applied:    true,
		workers:    7,
		shapeOCPUs: 6.5,
	})

	entry := requireSingleEntry(t, observed, zapcore.InfoLevel)

	if entry.Message != "sized worker pool from shape config" {
		t.Fatalf("unexpected log message %q", entry.Message)
	}

	if got, ok := fieldInt(entry.Context, "workerCount"); !ok || int(got) != 7 {
		t.Fatalf("expected workerCount field 7, got %v (present=%v)", got, ok)
	}

	requireLogFieldFloat(t, entry, "shapeOCPUs", 6.5)

	if got, ok := fieldInt(entry.Context, "workerCapMin"); !ok || int(got) != minAutoSizedWorkers {
		t.Fatalf("expected workerCapMin %d, got %v (present=%v)", minAutoSizedWorkers, got, ok)
	}

	if got, ok := fieldInt(entry.Context, "workerCapMax"); !ok || int(got) != maxAutoSizedWorkers {
		t.Fatalf("expected workerCapMax %d, got %v (present=%v)", maxAutoSizedWorkers, got, ok)
	}

	if hasField(entry.Context, "capped") {
		t.Fatalf("expected capped field to be absent, got %+v", entry.Context)
	}
}

func TestLogPoolSizingMarksCapped(t *testing.T) {
	t.Parallel()

	logger, observed := observer.New(zapcore.InfoLevel)

	logPoolSizing(zap.New(logger), poolSizingResult{
		applied:    true,
		workers:    maxAutoSizedWorkers,
		capped:     true,
		shapeOCPUs: 48,
	})

	entry := requireSingleEntry(t, observed, zapcore.InfoLevel)

	if capped, ok := fieldBool(entry.Context, "capped"); !ok || !capped {
		t.Fatalf("expected capped field to be true, got %v (present=%v)", capped, ok)
	}
}
