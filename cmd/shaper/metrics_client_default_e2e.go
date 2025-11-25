//go:build e2e

package main

import (
	"fmt"
	"os"
	"strings"

	"oci-cpu-shaper/internal/e2eclient"
	"oci-cpu-shaper/pkg/oci/metricsclient"
)

var newE2EMonitoringClient = func(endpoint string) (metricsclient.MetricsClient, error) {
	return e2eclient.NewMonitoringClient(endpoint)
}

func defaultMetricsClientBuilder() metricsclient.Builder {
	return func(compartmentID, region string) (metricsclient.MetricsClient, error) {
		endpoint := strings.TrimSpace(os.Getenv(e2eclient.MonitoringEndpointEnv))
		if endpoint != "" {
			client, err := newE2EMonitoringClient(endpoint)
			if err != nil {
				return nil, fmt.Errorf("build e2e monitoring client: %w", err)
			}

			return client, nil
		}

		builder := newInstancePrincipalBuilder.load()()

		return builder(compartmentID, region)
	}
}
