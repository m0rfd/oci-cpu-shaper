package metricsserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	ReadHeaderTimeout = 5 * time.Second
	ShutdownTimeout   = 5 * time.Second
)

var (
	ErrContextRequired = errors.New("metrics server: context is required")
	ErrServerDisabled  = errors.New("metrics server: disabled")
)

type ShutdownFunc func(context.Context)

type listenFunc func(ctx context.Context, network, address string) (net.Listener, error)

//
//nolint:gochecknoglobals // injected in tests
var listenEndpoint listenFunc = func(ctx context.Context, network, address string) (net.Listener, error) {
	var listenCfg net.ListenConfig

	return listenCfg.Listen(ctx, network, address)
}

// SetListenEndpointForTest replaces the listener factory during tests and returns a restore function.
func SetListenEndpointForTest(
	stub func(context.Context, string, string) (net.Listener, error),
) func() {
	previous := listenEndpoint
	listenEndpoint = stub

	return func() {
		listenEndpoint = previous
	}
}

type EndpointDeps struct {
	StartServer func(ctx context.Context, logger *zap.Logger, addr string, handler http.Handler) (ShutdownFunc, error)
}

func StartEndpoint(
	ctx context.Context,
	deps EndpointDeps,
	logger *zap.Logger,
	bindAddr string,
	handler http.Handler,
) (ShutdownFunc, context.CancelFunc, error) {
	if handler == nil {
		return nil, nil, nil
	}

	if deps.StartServer == nil {
		logger.Warn("metrics server disabled", zap.String("reason", "start function missing"))

		return nil, nil, nil
	}

	trimmed := strings.TrimSpace(bindAddr)
	if trimmed == "" {
		logger.Info("metrics server disabled", zap.String("reason", "http bind address empty"))

		return nil, nil, nil
	}

	if ctx == nil {
		return nil, nil, ErrContextRequired
	}

	logger.Info("starting metrics server", zap.String("bind", trimmed))

	metricsCtx, cancel := context.WithCancel(ctx)

	shutdown, err := deps.StartServer(metricsCtx, logger, trimmed, handler)
	if err != nil {
		cancel()

		return nil, nil, err
	}

	return shutdown, cancel, nil
}

func StartServer( //nolint:cyclop,funlen // listener lifecycle requires several guard branches
	ctx context.Context,
	logger *zap.Logger,
	addr string,
	handler http.Handler,
) (ShutdownFunc, error) {
	trimmed := strings.TrimSpace(addr)

	if logger == nil {
		logger = zap.NewNop()
	}

	if trimmed == "" {
		logger.Info("metrics server disabled", zap.String("reason", "http bind address empty"))

		return nil, ErrServerDisabled
	}

	if handler == nil {
		logger.Warn("metrics server disabled", zap.String("reason", "handler missing"))

		return nil, ErrServerDisabled
	}

	if ctx == nil {
		return nil, ErrContextRequired
	}

	baseCtx := context.WithoutCancel(ctx)

	listener, err := listenEndpoint(ctx, "tcp", trimmed)
	if err != nil {
		logger.Error(
			"metrics server listen failed",
			zap.String("bind", trimmed),
			zap.Error(err),
		)

		return nil, fmt.Errorf("listen metrics endpoint %q: %w", trimmed, err)
	}

	server := &http.Server{ //nolint:exhaustruct // only security-critical timeout configured here
		ReadHeaderTimeout: ReadHeaderTimeout,
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

		shutdownCtx, cancel := context.WithTimeout(baseCtx, ShutdownTimeout)
		defer cancel()

		shutdown(shutdownCtx)
	}()

	return func(shutdownCtx context.Context) {
		shutdown(shutdownCtx)

		<-serveDone
	}, nil
}
