//go:build !e2e

package main

import "oci-cpu-shaper/pkg/oci/metricsclient"

func defaultMetricsClientBuilder() metricsclient.Builder {
	return newInstancePrincipalBuilder()
}
