//nolint:testpackage // exercising context plumbing against unexported builders.
package metricsclient

import (
	"context"
	"errors"
	"testing"
)

type contextMarkerKey string

var (
	errFallbackInvoked = errors.New("fallback invoked")
	errFallback        = errors.New("fallback")
)

func TestWithBuilderNilContextUsesBackground(t *testing.T) {
	t.Parallel()

	var nilContext context.Context

	ctx := WithBuilder(nilContext, nil)
	if ctx != context.Background() {
		t.Fatalf("expected background context, got %v", ctx)
	}

	ctxWithMarker := context.WithValue(ctx, contextMarkerKey("marker"), "preserved")
	if got := ctxWithMarker.Value(contextMarkerKey("marker")); got != "preserved" {
		t.Fatalf("expected marker to be set on derived context, got %v", got)
	}
}

func TestWithBuilderNilFactoryUsesFallback(t *testing.T) {
	t.Parallel()

	original := context.WithValue(context.Background(), contextMarkerKey("marker"), "value")

	fallback := func(string, string) (MetricsClient, error) {
		return nil, errFallbackInvoked
	}

	ctx := WithBuilder(original, nil)
	if ctx != original {
		t.Fatal("expected context to be returned unchanged when builder is nil")
	}

	if ctx.Value(contextMarkerKey("marker")) != "value" {
		t.Fatalf(
			"expected marker to persist on unchanged context, got %v",
			ctx.Value(contextMarkerKey("marker")),
		)
	}

	builder := FromContext(ctx, fallback)

	_, err := builder("ocid.compartment", "us-test-1")
	if err == nil {
		t.Fatal("expected fallback builder to propagate error")
	}
}

func TestFromContextUsesStoredBuilder(t *testing.T) {
	t.Parallel()

	stub := new(stubMetricsAdapter)
	ctx := WithBuilder(
		context.Background(),
		func(compartmentID, region string) (MetricsClient, error) {
			if compartmentID != "ocid.compartment" {
				t.Fatalf("unexpected compartment %q", compartmentID)
			}

			if region != "us-test-1" {
				t.Fatalf("unexpected region %q", region)
			}

			return stub, nil
		},
	)

	builder := FromContext(ctx, nil)

	client, err := builder("ocid.compartment", "us-test-1")
	if err != nil {
		t.Fatalf("builder returned error: %v", err)
	}

	if client != stub {
		t.Fatalf("expected stored builder to be used, got %T", client)
	}
}

func TestFromContextDefaultsWhenMissing(t *testing.T) {
	t.Parallel()

	builder := FromContext(context.Background(), func(string, string) (MetricsClient, error) {
		return nil, errFallback
	})

	_, err := builder("ocid.compartment", "us-test-1")
	if err == nil {
		t.Fatal("expected fallback builder to propagate error")
	}
}

func TestFromContextSkipsNilBuilder(t *testing.T) {
	t.Parallel()

	called := 0
	builder := FromContext(
		context.WithValue(context.Background(), builderKey{}, Builder(nil)),
		func(string, string) (MetricsClient, error) {
			called++

			return nil, errFallback
		},
	)

	_, err := builder("ocid.compartment", "us-test-1")
	if err == nil {
		t.Fatal("expected fallback builder to propagate error")
	}

	if called != 1 {
		t.Fatalf("expected fallback builder to be invoked once, got %d", called)
	}
}
