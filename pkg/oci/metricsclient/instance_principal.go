package metricsclient

import (
	"context"
	"fmt"

	"oci-cpu-shaper/pkg/oci"
)

type p95CPUQuerier interface {
	QueryP95CPU(ctx context.Context, resourceID string, last7d bool) (float32, error)
}

type instancePrincipalConstructor func(compartmentID, region string, factory *oci.ClientFactory) (p95CPUQuerier, error)

type instancePrincipalOptions struct {
	constructor instancePrincipalConstructor
	factory     *oci.ClientFactory
}

// Option configures the InstancePrincipalBuilder.
type Option func(*instancePrincipalOptions)

// WithConstructor overrides the OCI client constructor used by the builder.
func WithConstructor(constructor instancePrincipalConstructor) Option {
	return func(opts *instancePrincipalOptions) {
		opts.constructor = constructor
	}
}

// WithFactory injects the ClientFactory used to construct the OCI Monitoring client.
func WithFactory(factory *oci.ClientFactory) Option {
	return func(opts *instancePrincipalOptions) {
		opts.factory = factory
	}
}

// InstancePrincipalBuilder returns a MetricsClient builder that wraps the OCI instance principal client.
func InstancePrincipalBuilder(opts ...Option) Builder {
	resolved := resolveOptions(opts)

	return func(compartmentID, region string) (oci.MetricsClient, error) {
		factory := resolved.factory
		if factory == nil {
			factory = oci.NewClientFactory()
		}

		constructor := resolved.constructor
		if constructor == nil {
			constructor = defaultInstancePrincipalConstructor
		}

		client, err := constructor(compartmentID, region, factory)
		if err != nil {
			return nil, err
		}

		return &instancePrincipalMetricsClient{client: client}, nil
	}
}

func resolveOptions(opts []Option) instancePrincipalOptions {
	var resolved instancePrincipalOptions

	for _, opt := range opts {
		if opt == nil {
			continue
		}

		opt(&resolved)
	}

	return resolved
}

//nolint:ireturn // interface return preserves client swap seams for tests and adapters.
func defaultInstancePrincipalConstructor(
	compartmentID, region string,
	factory *oci.ClientFactory,
) (p95CPUQuerier, error) {
	client, err := oci.NewInstancePrincipalClient(
		compartmentID,
		region,
		oci.WithFactory(factory),
	)
	if err != nil {
		return nil, fmt.Errorf("new instance principal client: %w", err)
	}

	return client, nil
}

type instancePrincipalMetricsClient struct {
	client p95CPUQuerier
}

func (m *instancePrincipalMetricsClient) QueryP95CPU(
	ctx context.Context,
	resourceID string,
) (float64, error) {
	if m == nil || m.client == nil {
		return 0, fmt.Errorf("query p95 cpu: %w", ErrDelegateNil)
	}

	value, err := m.client.QueryP95CPU(ctx, resourceID, true)
	if err != nil {
		return 0, fmt.Errorf("query p95 cpu: %w", err)
	}

	return float64(value), nil
}

type wrappedError string

func (w wrappedError) Error() string {
	return string(w)
}

var ErrDelegateNil error = wrappedError("metricsclient: delegate client is nil")
