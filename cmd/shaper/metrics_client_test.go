package main

import (
	"context"
	"errors"
	"testing"

	"oci-cpu-shaper/pkg/oci"
)

var (
	errStubPrincipal    = errors.New("stub: principal client")
	errStubQueryFailure = errors.New("stub: query failure")
)

type contextMarkerKey string

func TestBuildInstancePrincipalMetricsClientUsesFactory(t *testing.T) {
	t.Parallel()

	previousFactory := newInstancePrincipalClient

	t.Cleanup(func() {
		newInstancePrincipalClient = previousFactory
	})

	querier := newStubP95Querier(12.5, nil)

	var (
		receivedCompartment string
		receivedRegion      string
	)

	newInstancePrincipalClient = func(compartmentID, region string) (p95CPUQuerier, error) {
		receivedCompartment = compartmentID
		receivedRegion = region

		return querier, nil
	}

	client, err := buildInstancePrincipalMetricsClient("ocid.compartment", "us-test-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	typed, ok := client.(*instancePrincipalMetricsClient)
	if !ok {
		t.Fatalf("expected instancePrincipalMetricsClient, got %T", client)
	}

	if typed.client != querier {
		t.Fatalf("expected factory result to be wrapped, got %v", typed.client)
	}

	if receivedCompartment != "ocid.compartment" {
		t.Fatalf("expected compartment to propagate, got %q", receivedCompartment)
	}

	if receivedRegion != "us-test-1" {
		t.Fatalf("expected region to propagate, got %q", receivedRegion)
	}
}

//nolint:paralleltest // modifies global factory for controlled coverage.
func TestBuildInstancePrincipalMetricsClientPropagatesError(t *testing.T) {
	previousFactory := newInstancePrincipalClient

	t.Cleanup(func() {
		newInstancePrincipalClient = previousFactory
	})

	newInstancePrincipalClient = func(string, string) (p95CPUQuerier, error) {
		return nil, errStubPrincipal
	}

	_, err := buildInstancePrincipalMetricsClient("ocid.compartment", "us-test-1")
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, errStubPrincipal) {
		t.Fatalf("expected errStubPrincipal, got %v", err)
	}
}

func TestInstancePrincipalMetricsClientNilReceiver(t *testing.T) {
	t.Parallel()

	var client *instancePrincipalMetricsClient

	_, err := client.QueryP95CPU(context.Background(), "ocid.instance")
	if err == nil {
		t.Fatal("expected error for nil receiver")
	}

	if !errors.Is(err, errMetricsDelegateNil) {
		t.Fatalf("expected errMetricsDelegateNil, got %v", err)
	}
}

func TestInstancePrincipalMetricsClientNilDelegate(t *testing.T) {
	t.Parallel()

	client := &instancePrincipalMetricsClient{client: nil}

	_, err := client.QueryP95CPU(context.Background(), "ocid.instance")
	if err == nil {
		t.Fatal("expected error for nil delegate")
	}

	if !errors.Is(err, errMetricsDelegateNil) {
		t.Fatalf("expected errMetricsDelegateNil, got %v", err)
	}
}

func TestInstancePrincipalMetricsClientDelegateError(t *testing.T) {
	t.Parallel()

	querier := newStubP95Querier(0, errStubQueryFailure)
	client := &instancePrincipalMetricsClient{client: querier}

	_, err := client.QueryP95CPU(context.Background(), "ocid.instance")
	if err == nil {
		t.Fatal("expected delegated error")
	}

	if !errors.Is(err, errStubQueryFailure) {
		t.Fatalf("expected errStubQueryFailure, got %v", err)
	}

	if querier.calls != 1 {
		t.Fatalf("expected delegate to be invoked once, got %d", querier.calls)
	}

	if querier.lastResource != "ocid.instance" {
		t.Fatalf("expected resource to propagate, got %q", querier.lastResource)
	}

	if !querier.lastLast7d {
		t.Fatal("expected last7d flag to be true")
	}
}

type stubP95Querier struct {
	value        float32
	err          error
	calls        int
	lastResource string
	lastLast7d   bool
}

func (s *stubP95Querier) QueryP95CPU(
	_ context.Context,
	resourceID string,
	last7d bool,
) (float32, error) {
	s.calls++
	s.lastResource = resourceID
	s.lastLast7d = last7d

	if s.err != nil {
		return 0, s.err
	}

	return s.value, nil
}

func newStubP95Querier(value float32, err error) *stubP95Querier {
	return &stubP95Querier{value: value, err: err} //nolint:exhaustruct
}

func TestInstancePrincipalMetricsClientSuccess(t *testing.T) {
	t.Parallel()

	querier := newStubP95Querier(7.5, nil)
	client := &instancePrincipalMetricsClient{client: querier}

	value, err := client.QueryP95CPU(context.Background(), "ocid.instance")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value != float64(querier.value) {
		t.Fatalf("unexpected value: got %.2f want %.2f", value, querier.value)
	}

	if querier.calls != 1 {
		t.Fatalf("expected delegate to be called once, got %d", querier.calls)
	}

	if querier.lastResource != "ocid.instance" {
		t.Fatalf("expected resource to propagate, got %q", querier.lastResource)
	}

	if !querier.lastLast7d {
		t.Fatal("expected last7d flag to be true")
	}
}

func TestWithMetricsClientFactoryNilContext(t *testing.T) {
	t.Parallel()

	var nilContext context.Context

	ctx := withMetricsClientFactory(nilContext, nil)
	if ctx == nil {
		t.Fatal("expected background context when nil is provided")
	}
}

func TestWithMetricsClientFactoryNilFactoryReturnsOriginal(t *testing.T) {
	t.Parallel()

	original := context.WithValue(context.Background(), contextMarkerKey("marker"), "value")

	ctx := withMetricsClientFactory(original, nil)
	if ctx != original {
		t.Fatal("expected context to be returned unchanged when factory is nil")
	}
}

func TestMetricsClientFactoryFromContextUsesStoredFactory(t *testing.T) {
	t.Parallel()

	stub := new(stubMetricsAdapter)
	ctx := withMetricsClientFactory(
		context.Background(),
		func(compartmentID, region string) (oci.MetricsClient, error) {
			if compartmentID != "ocid.compartment" {
				t.Fatalf("unexpected compartment %q", compartmentID)
			}

			if region != "us-test-1" {
				t.Fatalf("unexpected region %q", region)
			}

			return stub, nil
		},
	)

	factory := metricsClientFactoryFromContext(ctx)

	client, err := factory("ocid.compartment", "us-test-1")
	if err != nil {
		t.Fatalf("factory returned error: %v", err)
	}

	if client != stub {
		t.Fatalf("expected stored factory to be used, got %T", client)
	}
}

//nolint:paralleltest // mutates global factory seams.
func TestMetricsClientFactoryFromContextDefaultsWhenMissing(t *testing.T) {
	previous := newInstancePrincipalClient

	t.Cleanup(func() {
		newInstancePrincipalClient = previous
	})

	newInstancePrincipalClient = func(string, string) (p95CPUQuerier, error) {
		return nil, errStubPrincipal
	}

	factory := metricsClientFactoryFromContext(context.Background())

	_, err := factory("ocid.compartment", "us-test-1")
	if err == nil {
		t.Fatal("expected default factory to propagate error")
	}

	if !errors.Is(err, errStubPrincipal) {
		t.Fatalf("expected errStubPrincipal, got %v", err)
	}
}

//nolint:paralleltest // mutates global factory seams.
func TestMetricsClientFactoryFromContextSkipsNilValue(t *testing.T) {
	previous := newInstancePrincipalClient

	t.Cleanup(func() {
		newInstancePrincipalClient = previous
	})

	called := 0
	newInstancePrincipalClient = func(string, string) (p95CPUQuerier, error) {
		called++

		return nil, errStubPrincipal
	}

	base := context.WithValue(
		context.Background(),
		metricsClientFactoryKey{},
		metricsClientFactory(nil),
	)
	factory := metricsClientFactoryFromContext(base)

	_, err := factory("ocid.compartment", "us-test-1")
	if err == nil {
		t.Fatal("expected default factory to propagate error")
	}

	if !errors.Is(err, errStubPrincipal) {
		t.Fatalf("expected errStubPrincipal, got %v", err)
	}

	if called != 1 {
		t.Fatalf("expected fallback factory to be invoked once, got %d", called)
	}
}
