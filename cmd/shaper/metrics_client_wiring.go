package main

import (
	"context"
	"fmt"

	"oci-cpu-shaper/internal/metricsserver"
	"oci-cpu-shaper/pkg/oci"
	"oci-cpu-shaper/pkg/oci/metricsclient"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

var (
	errMetricsContextRequired = metricsserver.ErrContextRequired
	errMetricsServerDisabled  = metricsserver.ErrServerDisabled
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
