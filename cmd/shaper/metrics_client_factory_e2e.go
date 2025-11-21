//go:build e2e

package main

import (
	"errors"
	"os"

	"github.com/oracle/oci-go-sdk/v65/common"

	"oci-cpu-shaper/pkg/oci"
)

const metricsFactoryFailureEnv = "OCI_CPU_SHAPER_E2E_FAIL_METRICS_CLIENT"

var errE2EMetricsFactoryFailure = errors.New("e2e: forced metrics factory failure")

func init() {
	if os.Getenv(metricsFactoryFailureEnv) == "" {
		return
	}

	newInstancePrincipalClientFactory = func() *oci.ClientFactory {
		return &oci.ClientFactory{ //nolint:exhaustruct
			InstancePrincipalProvider: func() (common.ConfigurationProvider, error) {
				return nil, errE2EMetricsFactoryFailure
			},
		}
	}
}
