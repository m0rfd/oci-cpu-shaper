//nolint:testpackage // constructor seams are exercised from within the package to cover internal options.
package metricsclient

import (
	"context"
	"errors"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
	"oci-cpu-shaper/pkg/oci"
)

var errStubQueryFailure = errors.New("stub: query failure")

func TestInstancePrincipalBuilderUsesConstructor(t *testing.T) {
	t.Parallel()

	stubQuerier := newStubP95Querier(12.5, nil)
	expectedFactory := oci.NewClientFactory()

	builder := InstancePrincipalBuilder(WithConstructor(func(
		compartmentID, region string,
		factory *oci.ClientFactory,
	) (p95CPUQuerier, error) {
		if compartmentID != "ocid.compartment" {
			t.Fatalf("unexpected compartment %q", compartmentID)
		}

		if region != "us-test-1" {
			t.Fatalf("unexpected region %q", region)
		}

		if factory != expectedFactory {
			t.Fatalf("expected injected factory, got %p", factory)
		}

		return stubQuerier, nil
	}), WithFactory(expectedFactory))

	client, err := builder("ocid.compartment", "us-test-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	typed, ok := client.(*instancePrincipalMetricsClient)
	if !ok {
		t.Fatalf("expected instancePrincipalMetricsClient, got %T", client)
	}

	if typed.client != stubQuerier {
		t.Fatalf("expected constructor result to be wrapped, got %v", typed.client)
	}
}

func TestInstancePrincipalBuilderDefaultsFactory(t *testing.T) {
	t.Parallel()

	builder := InstancePrincipalBuilder(WithConstructor(func(
		_ string,
		_ string,
		factory *oci.ClientFactory,
	) (p95CPUQuerier, error) {
		if factory == nil {
			t.Fatal("expected default factory")
		}

		return newStubP95Querier(0, nil), nil
	}))

	_, err := builder("ocid.compartment", "us-test-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstancePrincipalMetricsClientNilReceiver(t *testing.T) {
	t.Parallel()

	var client *instancePrincipalMetricsClient

	_, err := client.QueryP95CPU(context.Background(), "ocid.instance")
	if err == nil {
		t.Fatal("expected error for nil receiver")
	}

	if !errors.Is(err, ErrDelegateNil) {
		t.Fatalf("expected ErrDelegateNil, got %v", err)
	}
}

func TestInstancePrincipalMetricsClientNilDelegate(t *testing.T) {
	t.Parallel()

	client := &instancePrincipalMetricsClient{client: nil}

	_, err := client.QueryP95CPU(context.Background(), "ocid.instance")
	if err == nil {
		t.Fatal("expected error for nil delegate")
	}

	if !errors.Is(err, ErrDelegateNil) {
		t.Fatalf("expected ErrDelegateNil, got %v", err)
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
}

func TestInstancePrincipalBuilderConstructorError(t *testing.T) {
	t.Parallel()

	builder := InstancePrincipalBuilder(
		WithConstructor(func(string, string, *oci.ClientFactory) (p95CPUQuerier, error) {
			return nil, errStubQueryFailure
		}),
	)

	_, err := builder("ocid.compartment", "us-test-1")
	if err == nil {
		t.Fatal("expected constructor error to propagate")
	}

	if !errors.Is(err, errStubQueryFailure) {
		t.Fatalf("expected errStubQueryFailure, got %v", err)
	}
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
}

func TestInstancePrincipalBuilderUsesDefaultConstructor(t *testing.T) {
	t.Parallel()

	called := 0
	factory := &oci.ClientFactory{ //nolint:exhaustruct
		InstancePrincipalProvider: func() (common.ConfigurationProvider, error) {
			return common.NewRawConfigurationProvider("", "", "", "", "", nil), nil
		},
		MonitoringClient: func(common.ConfigurationProvider) (monitoring.MonitoringClient, error) {
			called++

			return monitoring.MonitoringClient{
				BaseClient: common.BaseClient{
					HTTPClient:  nil,
					Signer:      nil,
					Interceptor: nil,
					Host:        "",
					UserAgent:   "",
					BasePath:    "",
					Configuration: common.CustomClientConfiguration{
						RetryPolicy:    nil,
						CircuitBreaker: nil,
						RealmSpecificServiceEndpointTemplateEnabled: nil,
						EnableDualStackEndpoints:                    nil,
						ServiceUsesDualStackByDefault:               nil,
					},
				},
			}, nil
		},
	}

	builder := InstancePrincipalBuilder(nil, WithFactory(factory))

	client, err := builder("ocid.compartment", "us-test-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	typed, ok := client.(*instancePrincipalMetricsClient)
	if !ok {
		t.Fatalf("expected instancePrincipalMetricsClient, got %T", client)
	}

	if typed.client == nil {
		t.Fatal("expected OCI client to be wrapped by default constructor")
	}

	if called != 1 {
		t.Fatalf("expected monitoring client constructor to be invoked, got %d", called)
	}
}

func TestDefaultInstancePrincipalConstructorError(t *testing.T) {
	t.Parallel()

	factory := &oci.ClientFactory{ //nolint:exhaustruct
		InstancePrincipalProvider: func() (common.ConfigurationProvider, error) {
			return common.NewRawConfigurationProvider("", "", "", "", "", nil), nil
		},
		MonitoringClient: func(common.ConfigurationProvider) (monitoring.MonitoringClient, error) {
			return monitoring.MonitoringClient{
				BaseClient: common.BaseClient{
					HTTPClient:  nil,
					Signer:      nil,
					Interceptor: nil,
					Host:        "",
					UserAgent:   "",
					BasePath:    "",
					Configuration: common.CustomClientConfiguration{
						RetryPolicy:    nil,
						CircuitBreaker: nil,
						RealmSpecificServiceEndpointTemplateEnabled: nil,
						EnableDualStackEndpoints:                    nil,
						ServiceUsesDualStackByDefault:               nil,
					},
				},
			}, errStubQueryFailure
		},
	}

	_, err := defaultInstancePrincipalConstructor("ocid.compartment", "us-test-1", factory)
	if err == nil {
		t.Fatal("expected constructor error to propagate")
	}

	if !errors.Is(err, errStubQueryFailure) {
		t.Fatalf("expected errStubQueryFailure, got %v", err)
	}
}
