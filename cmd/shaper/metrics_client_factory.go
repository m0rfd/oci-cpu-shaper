package main

import "oci-cpu-shaper/pkg/oci"

//nolint:gochecknoglobals // test seams rely on substituting the constructor.
var newInstancePrincipalClientFactory = oci.NewClientFactory

//nolint:gochecknoglobals // test seams rely on substituting the constructor.
var newInstancePrincipalClient = func(compartmentID, region string) (p95CPUQuerier, error) {
	return oci.NewInstancePrincipalClient(
		compartmentID,
		region,
		oci.WithFactory(newInstancePrincipalClientFactory()),
	)
}
