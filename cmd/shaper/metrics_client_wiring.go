package main

import (
	"context"
	"errors"
	"fmt"

	"oci-cpu-shaper/pkg/oci"
	"oci-cpu-shaper/pkg/oci/metricsclient"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

var (
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

	builder := metricsclient.FromContext(ctx, defaultMetricsClientBuilder())

	metricsClient, err := builder(compartmentID, region)
	if err != nil {
		return nil, fmt.Errorf("build monitoring client: %w", err)
	}

	return metricsClient, nil
}
