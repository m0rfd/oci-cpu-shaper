//go:build e2e

package main

import (
	"fmt"
	"os"
	"strings"

	"oci-cpu-shaper/internal/e2eclient"
	"oci-cpu-shaper/pkg/oci/metricsclient"
)

func defaultMetricsClientBuilder() metricsclient.Builder {
	return func(compartmentID, region string) (metricsclient.MetricsClient, error) {
		endpoint := strings.TrimSpace(os.Getenv(e2eclient.MonitoringEndpointEnv))
		if endpoint != "" {
			client, err := e2eclient.NewMonitoringClient(endpoint)
			if err != nil {
				return nil, fmt.Errorf("build e2e monitoring client: %w", err)
			}

			return client, nil
		}

		return metricsclient.InstancePrincipalBuilder()(compartmentID, region)
	}
}
