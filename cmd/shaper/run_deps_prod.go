//go:build !e2e

package main

import (
	"os"

	"oci-cpu-shaper/internal/buildinfo"
	"oci-cpu-shaper/pkg/cgroup"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

func defaultRunDeps() runDeps {
	return runDeps{
		newLogger:          newLogger,
		newIMDS:            defaultIMDSFactory,
		newController:      defaultControllerFactory,
		currentBuildInfo:   buildinfo.Current,
		loadConfig:         runtimeconfig.Load,
		newMetricsExporter: metricshttp.NewExporter,
		startMetricsServer: startMetricsServer,
		versionWriter:      os.Stdout,
		detectCgroup: func() (*cgroup.CPU, error) {
			var reader cgroup.Reader

			return reader.Detect()
		},
	}
}
