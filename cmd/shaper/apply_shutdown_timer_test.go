package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type shutdownTimerContextKey string

func TestApplyShutdownTimerNonPositiveTimeout(t *testing.T) {
	t.Parallel()

	baseCtx := context.WithValue(context.Background(), shutdownTimerContextKey("base"), "base")

	t.Run("zero timeout returns original context", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := applyShutdownTimer(baseCtx, 0)

		if ctx != baseCtx {
			t.Fatalf("expected original context, got %v", ctx)
		}

		if cancel != nil {
			t.Fatalf("expected nil cancel func for zero timeout, got %v", cancel)
		}
	})

	t.Run("negative timeout returns original context", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := applyShutdownTimer(baseCtx, -time.Second)

		if ctx != baseCtx {
			t.Fatalf("expected original context, got %v", ctx)
		}

		if cancel != nil {
			t.Fatalf("expected nil cancel func for negative timeout, got %v", cancel)
		}
	})
}

func TestApplyShutdownTimerWithDeadline(t *testing.T) {
	t.Parallel()

	timeout := 25 * time.Millisecond
	start := time.Now()

	ctx, cancel := applyShutdownTimer(context.Background(), timeout)
	if cancel == nil {
		t.Fatal("expected cancel func for positive timeout")
	}

	t.Cleanup(cancel)

	if ctx == context.Background() {
		t.Fatal("expected derived context for positive timeout")
	}

	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		t.Fatal("expected derived context to include deadline")
	}

	if deadline.Before(start) {
		t.Fatalf("expected deadline after start, got %v before %v", deadline, start)
	}

	if deadline.After(start.Add(timeout + 15*time.Millisecond)) {
		t.Fatalf("expected deadline near %v, got %v", start.Add(timeout), deadline)
	}

	select {
	case <-ctx.Done():
		err := ctx.Err()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded error, got %v", err)
		}
	case <-time.After(timeout + 100*time.Millisecond):
		t.Fatalf("context did not timeout within %v", timeout)
	}
}

func TestApplyShutdownTimerCancelStopsContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := applyShutdownTimer(context.Background(), 50*time.Millisecond)
	if cancel == nil {
		t.Fatal("expected cancel func for positive timeout")
	}

	cancel()

	select {
	case <-ctx.Done():
		err := ctx.Err()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled error, got %v", err)
		}
	case <-time.After(25 * time.Millisecond):
		t.Fatal("context not canceled after invoking cancel func")
	}
}
