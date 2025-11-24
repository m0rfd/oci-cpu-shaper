package main

import (
	"testing"

	"oci-cpu-shaper/pkg/oci/metricsclient"
)

func TestDefaultMetricsClientBuilderUsesInstancePrincipal(t *testing.T) {
	t.Parallel()

	stub := newStubMetricsClient()

	oldBuilder := newInstancePrincipalBuilder
	newInstancePrincipalBuilder = func(_ ...metricsclient.Option) metricsclient.Builder {
		t.Helper()

		return func(compartmentID, region string) (metricsclient.MetricsClient, error) {
			if compartmentID != testCompartmentID {
				t.Fatalf("expected compartment %q, got %q", testCompartmentID, compartmentID)
			}

			if region != testRegion {
				t.Fatalf("expected region %q, got %q", testRegion, region)
			}

			return stub, nil
		}
	}

	t.Cleanup(func() {
		newInstancePrincipalBuilder = oldBuilder
	})

	client, err := defaultMetricsClientBuilder()(testCompartmentID, testRegion)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client != stub {
		t.Fatalf("expected instance principal builder result, got %T", client)
	}
}
