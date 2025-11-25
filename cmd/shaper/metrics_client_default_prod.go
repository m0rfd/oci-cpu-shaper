//go:build !e2e

package main

import "oci-cpu-shaper/pkg/oci/metricsclient"

func defaultMetricsClientBuilder() metricsclient.Builder {
	factory := newInstancePrincipalBuilder.load()

	return factory()
}
