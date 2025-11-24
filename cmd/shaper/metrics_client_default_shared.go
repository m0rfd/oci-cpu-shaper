package main

import "oci-cpu-shaper/pkg/oci/metricsclient"

// newInstancePrincipalBuilder allows tests to inject a custom metrics builder.
var newInstancePrincipalBuilder = metricsclient.InstancePrincipalBuilder //nolint:gochecknoglobals
