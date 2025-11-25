package main

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"
	"oci-cpu-shaper/internal/metricsserver"
)

const metricsShutdownTimeout = metricsserver.ShutdownTimeout

type metricsShutdownFunc = metricsserver.ShutdownFunc

func startMetricsEndpoint(
	ctx context.Context,
	deps runDeps,
	logger *zap.Logger,
	bindAddr string,
	handler http.Handler,
) (metricsShutdownFunc, context.CancelFunc, error) {
	shutdown, cancel, err := metricsserver.StartEndpoint(
		ctx,
		metricsserver.EndpointDeps{StartServer: deps.startMetricsServer},
		logger,
		bindAddr,
		handler,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("start metrics endpoint: %w", err)
	}

	return shutdown, cancel, nil
}

func startMetricsServer(
	ctx context.Context,
	logger *zap.Logger,
	addr string,
	handler http.Handler,
) (metricsShutdownFunc, error) {
	shutdown, err := metricsserver.StartServer(ctx, logger, addr, handler)
	if err != nil {
		return nil, fmt.Errorf("start metrics server: %w", err)
	}

	return shutdown, nil
}
