package main

import (
	"context"
	"errors"
	"testing"

	"oci-cpu-shaper/pkg/oci/metricsclient"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

const (
	testCompartmentID = "ocid.compartment"
	testRegion        = "us-test-1"
)

var (
	errStubFactoryFailure = errors.New("stub: factory failure")
	errStubFactoryInvoked = errors.New("factory should not be invoked in offline mode")
)

func TestCreateMetricsClientOfflineUsesStaticSeed(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Config{ //nolint:exhaustruct // only controller seed is required
		Controller: runtimeconfig.ControllerConfig{ //nolint:exhaustruct // only target start is required
			TargetStart: 42.5,
		},
	}

	ctx := metricsclient.WithBuilder(
		context.Background(),
		func(string, string) (metricsclient.MetricsClient, error) {
			t.Fatalf("factory should not be called when offline")

			return nil, errStubFactoryInvoked
		},
	)

	client, err := createMetricsClient(ctx, cfg, true, "ignored", "ignored")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	value, err := client.QueryP95CPU(context.Background(), "ocid.instance")
	if err != nil {
		t.Fatalf("unexpected query error: %v", err)
	}

	if value != cfg.Controller.TargetStart {
		t.Fatalf(
			"expected static client seeded with %.2f, got %.2f",
			cfg.Controller.TargetStart,
			value,
		)
	}
}

func TestCreateMetricsClientOnlineUsesInjectedFactory(t *testing.T) {
	t.Parallel()

	stub := newStubMetricsClient()

	var (
		receivedCompartment string
		receivedRegion      string
		calls               int
	)

	ctx := metricsclient.WithBuilder(
		context.Background(),
		func(compartmentID, region string) (metricsclient.MetricsClient, error) {
			calls++
			receivedCompartment = compartmentID
			receivedRegion = region

			return stub, nil
		},
	)

	cfg := runtimeconfig.Config{} //nolint:exhaustruct // controller seed unused

	client, err := createMetricsClient(ctx, cfg, false, testCompartmentID, testRegion)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client != stub {
		t.Fatalf("expected injected factory result, got %T", client)
	}

	if calls != 1 {
		t.Fatalf("expected injected factory to be called once, got %d", calls)
	}

	if receivedCompartment != testCompartmentID {
		t.Fatalf("expected compartment to propagate, got %q", receivedCompartment)
	}

	if receivedRegion != testRegion {
		t.Fatalf("expected region to propagate, got %q", receivedRegion)
	}
}

func TestCreateMetricsClientOnlinePropagatesFactoryErrors(t *testing.T) {
	t.Parallel()

	ctx := metricsclient.WithBuilder(
		context.Background(),
		func(string, string) (metricsclient.MetricsClient, error) {
			return nil, errStubFactoryFailure
		},
	)

	cfg := runtimeconfig.Config{} //nolint:exhaustruct // controller seed unused

	_, err := createMetricsClient(ctx, cfg, false, testCompartmentID, testRegion)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, errStubFactoryFailure) {
		t.Fatalf("expected injected error, got %v", err)
	}
}
