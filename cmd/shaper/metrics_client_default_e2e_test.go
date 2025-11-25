//go:build e2e

package main

import (
	"errors"
	"os"
	"testing"

	"oci-cpu-shaper/internal/e2eclient"
	"oci-cpu-shaper/pkg/oci/metricsclient"
)

var errStubE2EMonitoring = errors.New("stub: monitoring client")

func TestDefaultMetricsClientBuilderE2EUsesMonitoringEndpoint(t *testing.T) {
	const endpoint = " http://example.test/monitoring "

	stub := newStubMetricsClient()

	oldE2E := newE2EMonitoringClient
	newE2EMonitoringClient = func(receivedEndpoint string) (metricsclient.MetricsClient, error) {
		if receivedEndpoint != "http://example.test/monitoring" {
			t.Fatalf("expected trimmed endpoint, got %q", receivedEndpoint)
		}

		return stub, nil
	}
	t.Cleanup(func() {
		newE2EMonitoringClient = oldE2E
	})

	t.Setenv(e2eclient.MonitoringEndpointEnv, endpoint)

	client, err := defaultMetricsClientBuilder()(testCompartmentID, testRegion)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client != stub {
		t.Fatalf("expected monitoring client result, got %T", client)
	}
}

func TestDefaultMetricsClientBuilderE2EPropagatesMonitoringErrors(t *testing.T) {
	oldE2E := newE2EMonitoringClient
	newE2EMonitoringClient = func(string) (metricsclient.MetricsClient, error) {
		return nil, errStubE2EMonitoring
	}
	t.Cleanup(func() {
		newE2EMonitoringClient = oldE2E
	})

	t.Setenv(e2eclient.MonitoringEndpointEnv, "http://example.test/monitoring")

	_, err := defaultMetricsClientBuilder()(testCompartmentID, testRegion)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, errStubE2EMonitoring) {
		t.Fatalf("expected monitoring error, got %v", err)
	}
}

func TestDefaultMetricsClientBuilderE2EFallsBackToInstancePrincipal(t *testing.T) {
	stub := newStubMetricsClient()

	oldBuilder := newInstancePrincipalBuilder.swap(func(opts ...metricsclient.Option) metricsclient.Builder {
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
	})
	t.Cleanup(func() {
		newInstancePrincipalBuilder.swap(oldBuilder)
	})

	os.Unsetenv(e2eclient.MonitoringEndpointEnv)

	client, err := defaultMetricsClientBuilder()(testCompartmentID, testRegion)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client != stub {
		t.Fatalf("expected instance principal fallback, got %T", client)
	}
}
