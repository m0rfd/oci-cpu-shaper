package oci //nolint:testpackage

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

func TestNewClientFactoryProvidesDefaults(t *testing.T) {
	t.Parallel()

	factory := NewClientFactory()

	requireFunctionPointerEqual(
		t,
		factory.InstancePrincipalProvider,
		auth.InstancePrincipalConfigurationProvider,
		"instance principal provider",
	)

	requireFunctionPointerEqual(
		t,
		factory.NewMonitoringClient,
		monitoring.NewMonitoringClientWithConfigurationProvider,
		"monitoring client constructor",
	)

	requireFunctionPointerEqual(t, factory.Now, time.Now, "clock function")
}

func TestClientFactoryWithDefaultsFillsZeroValue(t *testing.T) {
	t.Parallel()

	var zeroFactory ClientFactory

	factory := zeroFactory.withDefaults()

	requireFunctionPointerEqual(
		t,
		factory.InstancePrincipalProvider,
		auth.InstancePrincipalConfigurationProvider,
		"instance principal provider",
	)

	requireFunctionPointerEqual(
		t,
		factory.NewMonitoringClient,
		monitoring.NewMonitoringClientWithConfigurationProvider,
		"monitoring client constructor",
	)

	requireFunctionPointerEqual(t, factory.Now, time.Now, "clock function")
}

func TestWithFactoryOverridesOptions(t *testing.T) {
	t.Parallel()

	customFactory := ClientFactory{
		InstancePrincipalProvider: func() (common.ConfigurationProvider, error) {
			return stubConfigurationProvider(t), nil
		},
		NewMonitoringClient: func(common.ConfigurationProvider) (monitoring.MonitoringClient, error) {
			var client monitoring.MonitoringClient

			return client, nil
		},
		Now: func() time.Time {
			return time.Unix(42, 0).UTC()
		},
	}

	options := applyClientOptions([]ClientOption{WithFactory(customFactory)})

	requireFunctionPointerEqual(
		t,
		options.factory.InstancePrincipalProvider,
		customFactory.InstancePrincipalProvider,
		"instance principal provider",
	)

	requireFunctionPointerEqual(
		t,
		options.factory.NewMonitoringClient,
		customFactory.NewMonitoringClient,
		"monitoring client constructor",
	)

	requireFunctionPointerEqual(t, options.factory.Now, customFactory.Now, "clock function")
}

func TestNewInstancePrincipalClientPropagatesProviderError(t *testing.T) {
	t.Parallel()

	factory := ClientFactory{
		InstancePrincipalProvider: func() (common.ConfigurationProvider, error) {
			return nil, errForcedFailure
		},
		NewMonitoringClient: nil,
		Now:                 nil,
	}

	_, err := NewInstancePrincipalClient(
		"ocid1.compartment.oc1..exampleuniqueID",
		"us-ashburn-1",
		WithFactory(factory),
	)
	if err == nil || !strings.Contains(err.Error(), "build instance principal provider") {
		t.Fatalf("expected wrapped provider error, got %v", err)
	}
}

func TestNewInstancePrincipalClientPropagatesClientError(t *testing.T) {
	t.Parallel()

	provider := stubConfigurationProvider(t)

	factory := ClientFactory{
		InstancePrincipalProvider: func() (common.ConfigurationProvider, error) {
			return provider, nil
		},
		NewMonitoringClient: func(common.ConfigurationProvider) (monitoring.MonitoringClient, error) {
			var client monitoring.MonitoringClient

			return client, errForcedFailure
		},
		Now: nil,
	}

	_, err := NewInstancePrincipalClient(
		"ocid1.compartment.oc1..exampleuniqueID",
		"us-ashburn-1",
		WithFactory(factory),
	)
	if err == nil || !strings.Contains(err.Error(), "create monitoring client") {
		t.Fatalf("expected monitoring client error, got %v", err)
	}
}

func TestNewInstancePrincipalClientSuccess(t *testing.T) {
	t.Parallel()

	provider := stubConfigurationProvider(t)
	fakeNow := time.Date(2024, time.June, 30, 17, 0, 0, 0, time.UTC)

	factory := ClientFactory{
		InstancePrincipalProvider: func() (common.ConfigurationProvider, error) {
			return provider, nil
		},
		NewMonitoringClient: func(common.ConfigurationProvider) (monitoring.MonitoringClient, error) {
			var client monitoring.MonitoringClient

			return client, nil
		},
		Now: func() time.Time {
			return fakeNow
		},
	}

	client, err := NewInstancePrincipalClient(
		"ocid1.compartment.oc1..exampleuniqueID",
		"us-ashburn-1",
		WithFactory(factory),
	)
	requireNoError(t, err, "construct instance principal client")

	if client == nil {
		t.Fatalf("expected client instance")
	}

	requireEqual(
		t,
		client.compartmentID,
		"ocid1.compartment.oc1..exampleuniqueID",
		"compartment ID",
	)

	sdkClient, ok := client.metrics.(*sdkMonitoringClient)
	if !ok || sdkClient == nil || sdkClient.client == nil {
		t.Fatalf("expected sdkMonitoringClient, got %#v", client.metrics)
	}

	if actual := client.now(); !actual.Equal(fakeNow) {
		t.Fatalf("expected factory clock to be used, got %v", actual)
	}
}

func requireFunctionPointerEqual(t *testing.T, actual, expected any, description string) {
	t.Helper()

	if functionPointer(actual) != functionPointer(expected) {
		t.Fatalf("expected %s to match default", description)
	}
}

func functionPointer(fn any) uintptr {
	if fn == nil {
		return 0
	}

	return reflect.ValueOf(fn).Pointer()
}
