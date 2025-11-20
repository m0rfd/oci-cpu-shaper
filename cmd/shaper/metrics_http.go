package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/oci"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

const (
	metricsReadHeaderTimeout = 5 * time.Second
	metricsShutdownTimeout   = 5 * time.Second
)

type metricsClientFactory func(compartmentID, region string) (oci.MetricsClient, error)

type metricsClientFactoryKey struct{}

func withMetricsClientFactory(ctx context.Context, factory metricsClientFactory) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	if factory == nil {
		return ctx
	}

	return context.WithValue(ctx, metricsClientFactoryKey{}, factory)
}

func metricsClientFactoryFromContext(ctx context.Context) metricsClientFactory {
	if ctx != nil {
		if factory, ok := ctx.Value(metricsClientFactoryKey{}).(metricsClientFactory); ok &&
			factory != nil {
			return factory
		}
	}

	return buildInstancePrincipalMetricsClient
}

var (
	errMetricsDelegateNil     = errors.New("metrics client: nil delegate")
	errMetricsContextRequired = errors.New("metrics server: context is required")
	errMetricsServerDisabled  = errors.New("metrics server: disabled")
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

	factory := metricsClientFactoryFromContext(ctx)

	metricsClient, err := factory(compartmentID, region)
	if err != nil {
		return nil, fmt.Errorf("build monitoring client: %w", err)
	}

	return metricsClient, nil
}

func startMetricsServer( //nolint:cyclop,funlen // listener lifecycle requires several guard branches
	ctx context.Context,
	logger *zap.Logger,
	addr string,
	handler http.Handler,
) (metricsShutdownFunc, error) {
	trimmed := strings.TrimSpace(addr)

	if logger == nil {
		logger = zap.NewNop()
	}

	if trimmed == "" {
		logger.Info("metrics server disabled", zap.String("reason", "http bind address empty"))

		return nil, errMetricsServerDisabled
	}

	if handler == nil {
		logger.Warn("metrics server disabled", zap.String("reason", "handler missing"))

		return nil, errMetricsServerDisabled
	}

	if ctx == nil {
		return nil, errMetricsContextRequired
	}

	baseCtx := context.WithoutCancel(ctx)

	var listenCfg net.ListenConfig

	listener, err := listenCfg.Listen(ctx, "tcp", trimmed)
	if err != nil {
		logger.Error(
			"metrics server listen failed",
			zap.String("bind", trimmed),
			zap.Error(err),
		)

		return nil, fmt.Errorf("listen metrics endpoint %q: %w", trimmed, err)
	}

	server := &http.Server{ //nolint:exhaustruct // only security-critical timeout configured here
		ReadHeaderTimeout: metricsReadHeaderTimeout,
	}
	server.Addr = trimmed
	server.Handler = handler

	logger.Info("metrics server listening", zap.String("bind", trimmed))

	serveDone := make(chan struct{})

	go func() {
		defer close(serveDone)

		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("metrics server serve", zap.String("bind", trimmed), zap.Error(err))

			return
		}

		logger.Info("metrics server stopped", zap.String("bind", trimmed))
	}()

	shutdown := func(shutdownCtx context.Context) {
		if shutdownCtx == nil {
			shutdownCtx = baseCtx
		}

		logger.Info("stopping metrics server", zap.String("bind", trimmed))

		err := server.Shutdown(shutdownCtx)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Warn("metrics server shutdown", zap.String("bind", trimmed), zap.Error(err))
		}
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(baseCtx, metricsShutdownTimeout)
		defer cancel()

		shutdown(shutdownCtx)
	}()

	return func(shutdownCtx context.Context) {
		shutdown(shutdownCtx)

		<-serveDone
	}, nil
}

type p95CPUQuerier interface {
	QueryP95CPU(ctx context.Context, resourceID string, last7d bool) (float32, error)
}

type instancePrincipalMetricsClient struct {
	client p95CPUQuerier
}

func (m *instancePrincipalMetricsClient) QueryP95CPU(
	ctx context.Context,
	resourceID string,
) (float64, error) {
	if m == nil || m.client == nil {
		return 0, errMetricsDelegateNil
	}

	value, err := m.client.QueryP95CPU(ctx, resourceID, true)
	if err != nil {
		return 0, fmt.Errorf("query p95 cpu: %w", err)
	}

	return float64(value), nil
}
