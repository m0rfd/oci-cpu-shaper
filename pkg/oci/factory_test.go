package oci //nolint:testpackage

import (
	"fmt"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

func TestResolveFactoryDefaults(t *testing.T) {
	t.Parallel()

	defaults := NewClientFactory()
	res := resolveFactory(nil)

	if res.InstancePrincipalProvider == nil || res.MonitoringClient == nil || res.Clock == nil {
		t.Fatalf("expected non-nil defaults, got %#v", res)
	}

	if fmt.Sprintf(
		"%p",
		defaults.InstancePrincipalProvider,
	) != fmt.Sprintf(
		"%p",
		res.InstancePrincipalProvider,
	) {
		t.Fatalf("expected default instance principal provider to be preserved")
	}

	if fmt.Sprintf("%p", defaults.MonitoringClient) != fmt.Sprintf("%p", res.MonitoringClient) {
		t.Fatalf("expected default monitoring client constructor to be preserved")
	}

	if fmt.Sprintf("%p", defaults.Clock) != fmt.Sprintf("%p", res.Clock) {
		t.Fatalf("expected default clock to be preserved")
	}
}

func TestResolveFactoryIgnoresNilOptions(t *testing.T) {
	t.Parallel()

	defaults := NewClientFactory()
	res := resolveFactory([]ClientOption{nil})

	if res.InstancePrincipalProvider == nil || res.MonitoringClient == nil || res.Clock == nil {
		t.Fatalf("expected defaults when options are nil, got %#v", res)
	}

	if fmt.Sprintf(
		"%p",
		defaults.InstancePrincipalProvider,
	) != fmt.Sprintf(
		"%p",
		res.InstancePrincipalProvider,
	) {
		t.Fatalf(
			"expected default instance principal provider to be preserved when options are nil",
		)
	}

	if fmt.Sprintf("%p", defaults.MonitoringClient) != fmt.Sprintf("%p", res.MonitoringClient) {
		t.Fatalf(
			"expected default monitoring client constructor to be preserved when options are nil",
		)
	}

	if fmt.Sprintf("%p", defaults.Clock) != fmt.Sprintf("%p", res.Clock) {
		t.Fatalf("expected default clock to be preserved when options are nil")
	}
}

func TestResolveFactoryPartialOverrides(t *testing.T) {
	t.Parallel()

	provider := stubConfigurationProvider(t)

	res := resolveFactory([]ClientOption{
		WithFactory(
			&ClientFactory{InstancePrincipalProvider: func() (common.ConfigurationProvider, error) {
				return provider, nil
			}, MonitoringClient: nil, Clock: nil},
		),
	})

	if res.InstancePrincipalProvider == nil {
		t.Fatalf("expected overridden instance principal provider")
	}

	resolvedProvider, err := res.InstancePrincipalProvider()
	requireNoError(t, err, "resolve provider")

	if resolvedProvider != provider {
		t.Fatalf("expected custom provider to be used")
	}

	if res.MonitoringClient == nil {
		t.Fatalf("expected default monitoring client constructor to remain")
	}

	if res.Clock == nil {
		t.Fatalf("expected default clock to remain")
	}
}

func TestNewInstancePrincipalClientUsesFactoryClock(t *testing.T) {
	t.Parallel()

	expectedNow := time.Date(2024, time.July, 15, 12, 0, 0, 0, time.UTC)
	provider := stubConfigurationProvider(t)

	factory := &ClientFactory{
		InstancePrincipalProvider: func() (common.ConfigurationProvider, error) {
			return provider, nil
		},
		MonitoringClient: func(p common.ConfigurationProvider) (monitoring.MonitoringClient, error) {
			if p != provider {
				t.Fatalf("expected provider passthrough")
			}

			client, err := monitoring.NewMonitoringClientWithConfigurationProvider(p)
			requireNoError(t, err, "build monitoring client")

			return client, nil
		},
		Clock: func() time.Time { return expectedNow },
	}

	client, err := NewInstancePrincipalClient(
		"ocid1.compartment.oc1..exampleuniqueID",
		"us-phoenix-1",
		WithFactory(factory),
	)
	requireNoError(t, err, "create instance principal client with custom clock")

	if client == nil {
		t.Fatalf("expected client instance")
	}

	if got := client.now(); !got.Equal(expectedNow) {
		t.Fatalf("expected custom clock to be used, got %v want %v", got, expectedNow)
	}
}
