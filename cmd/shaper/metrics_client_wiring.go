package main

import (
	"context"
	"errors"
	"fmt"

	"oci-cpu-shaper/pkg/oci"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

type metricsClientFactory func(compartmentID, region string) (oci.MetricsClient, error)

type metricsClientFactoryKey struct{}

func withMetricsClientFactory(ctx context.Context, factory metricsClientFactory) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	if factory == nil {
		return ctx
	}

	return context.WithValue(ctx, metricsClientFactoryKey{}, factory)
}

func metricsClientFactoryFromContext(ctx context.Context) metricsClientFactory {
	if ctx != nil {
		if factory, ok := ctx.Value(metricsClientFactoryKey{}).(metricsClientFactory); ok &&
			factory != nil {
			return factory
		}
	}

	return buildInstancePrincipalMetricsClient
}

var (
	errMetricsDelegateNil     = errors.New("metrics client: nil delegate")
	errMetricsContextRequired = errors.New("metrics server: context is required")
	errMetricsServerDisabled  = errors.New("metrics server: disabled")
)

//nolint:ireturn // helper returns MetricsClient interface for dependency substitution.
func createMetricsClient(
	ctx context.Context,
	cfg runtimeconfig.Config,
	offline bool,
	compartmentID string,
	region string,
) (oci.MetricsClient, error) {
	if offline {
		return oci.NewStaticMetricsClient(cfg.Controller.TargetStart), nil
	}

	factory := metricsClientFactoryFromContext(ctx)

	metricsClient, err := factory(compartmentID, region)
	if err != nil {
		return nil, fmt.Errorf("build monitoring client: %w", err)
	}

	return metricsClient, nil
}

type p95CPUQuerier interface {
	QueryP95CPU(ctx context.Context, resourceID string, last7d bool) (float32, error)
}

type instancePrincipalMetricsClient struct {
	client p95CPUQuerier
}

func (m *instancePrincipalMetricsClient) QueryP95CPU(
	ctx context.Context,
	resourceID string,
) (float64, error) {
	if m == nil || m.client == nil {
		return 0, errMetricsDelegateNil
	}

	value, err := m.client.QueryP95CPU(ctx, resourceID, true)
	if err != nil {
		return 0, fmt.Errorf("query p95 cpu: %w", err)
	}

	return float64(value), nil
}
