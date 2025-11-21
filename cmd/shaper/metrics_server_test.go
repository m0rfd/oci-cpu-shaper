package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

const testMetricsBind = "127.0.0.1:0"

func TestStartMetricsEndpointSkipsWhenHandlerMissing(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		t.Fatal("expected startMetricsServer not to be called when handler is nil")

		return nil, errMetricsServerBoom
	}

	shutdown, cancel, err := startMetricsEndpoint(
		context.Background(),
		deps,
		zap.NewNop(),
		testMetricsBind,
		nil,
	)
	if err != nil {
		t.Fatalf("expected nil error when handler missing, got %v", err)
	}

	if shutdown != nil || cancel != nil {
		t.Fatalf("expected nil shutdown and cancel, got %v, %v", shutdown, cancel)
	}
}

func TestStartMetricsEndpointSkipsWhenStarterMissing(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	deps.startMetricsServer = nil

	handler := http.NewServeMux()
	handler.Handle("/metrics", http.NotFoundHandler())

	shutdown, cancel, err := startMetricsEndpoint(
		context.Background(),
		deps,
		zap.NewNop(),
		testMetricsBind,
		handler,
	)
	if err != nil {
		t.Fatalf("expected nil error when starter missing, got %v", err)
	}

	if shutdown != nil || cancel != nil {
		t.Fatalf("expected nil shutdown and cancel, got %v, %v", shutdown, cancel)
	}
}

func TestStartMetricsEndpointSkipsWhenBindAddressEmpty(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		t.Fatal("expected startMetricsServer not to be called when bind address is empty")

		return nil, errMetricsServerBoom
	}

	shutdown, cancel, err := startMetricsEndpoint(
		context.Background(),
		deps,
		zap.NewNop(),
		"   ",
		http.NewServeMux(),
	)
	if err != nil {
		t.Fatalf("expected nil error when bind address empty, got %v", err)
	}

	if shutdown != nil || cancel != nil {
		t.Fatalf("expected nil shutdown and cancel, got %v, %v", shutdown, cancel)
	}
}

func TestStartMetricsEndpointRequiresContext(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		t.Fatal("expected startMetricsServer not to be called when context is missing")

		return nil, errMetricsServerBoom
	}

	handler := http.NewServeMux()
	handler.Handle("/metrics", http.NotFoundHandler())

	var nilContext context.Context

	shutdown, cancel, err := startMetricsEndpoint(
		nilContext,
		deps,
		zap.NewNop(),
		testMetricsBind,
		handler,
	)
	if !errors.Is(err, errMetricsContextRequired) {
		t.Fatalf("expected errMetricsContextRequired, got %v", err)
	}

	if shutdown != nil || cancel != nil {
		t.Fatalf("expected nil shutdown and cancel, got %v, %v", shutdown, cancel)
	}
}

func TestStartMetricsEndpointStartsServer(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	handler := http.NewServeMux()
	handler.Handle("/metrics", http.NotFoundHandler())

	trimmedBind := strings.TrimSpace("  " + testMetricsBind + "  ")

	var (
		startAddr    string
		startLogger  *zap.Logger
		startHandler http.Handler
		startCalled  bool
	)

	deps.startMetricsServer = func(
		ctx context.Context,
		logger *zap.Logger,
		addr string,
		servedHandler http.Handler,
	) (metricsShutdownFunc, error) {
		if ctx == nil {
			t.Fatal("expected context to be provided")
		}

		startCalled = true
		startAddr = addr
		startLogger = logger
		startHandler = servedHandler

		return func(context.Context) {}, nil
	}

	shutdown, cancel, err := startMetricsEndpoint(
		context.Background(),
		deps,
		zap.NewNop(),
		"  "+testMetricsBind+"  ",
		handler,
	)
	if err != nil {
		t.Fatalf("startMetricsEndpoint returned error: %v", err)
	}

	assertMetricsEndpointStarted(
		t,
		shutdown,
		cancel,
		startCalled,
		startAddr,
		trimmedBind,
		startHandler,
		handler,
		startLogger,
	)
}

func TestStartMetricsEndpointCancelSignalsServer(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	handler := http.NewServeMux()
	handler.Handle("/metrics", http.NotFoundHandler())

	canceled := make(chan struct{})

	deps.startMetricsServer = func(
		ctx context.Context,
		_ *zap.Logger,
		_ string,
		_ http.Handler,
	) (metricsShutdownFunc, error) {
		go func() {
			<-ctx.Done()
			close(canceled)
		}()

		return func(context.Context) {}, nil
	}

	_, cancel, err := startMetricsEndpoint(
		context.Background(),
		deps,
		zap.NewNop(),
		testMetricsBind,
		handler,
	)
	if err != nil {
		t.Fatalf("startMetricsEndpoint returned error: %v", err)
	}

	cancel()

	select {
	case <-canceled:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected metrics context to be canceled after calling cancel")
	}
}

func TestStartMetricsEndpointPropagatesStartError(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	handler := http.NewServeMux()
	handler.Handle("/metrics", http.NotFoundHandler())

	canceled := make(chan struct{})

	deps.startMetricsServer = func(
		ctx context.Context,
		_ *zap.Logger,
		_ string,
		_ http.Handler,
	) (metricsShutdownFunc, error) {
		go func() {
			<-ctx.Done()
			close(canceled)
		}()

		return nil, errMetricsServerBoom
	}

	shutdown, cancel, err := startMetricsEndpoint(
		context.Background(),
		deps,
		zap.NewNop(),
		testMetricsBind,
		handler,
	)
	if !errors.Is(err, errMetricsServerBoom) {
		t.Fatalf("expected errMetricsServerBoom, got %v", err)
	}

	if shutdown != nil || cancel != nil {
		t.Fatalf("expected nil shutdown and cancel on failure, got %v, %v", shutdown, cancel)
	}

	select {
	case <-canceled:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected metrics context to be canceled when start fails")
	}
}

func TestStartMetricsServerSkipsWhenAddressOrHandlerMissing(t *testing.T) {
	t.Parallel()

	shutdown, err := startMetricsServer(
		context.Background(),
		zap.NewNop(),
		"   ",
		http.NewServeMux(),
	)
	if !errors.Is(err, errMetricsServerDisabled) {
		if err == nil {
			t.Fatal("expected errMetricsServerDisabled, got nil")
		}

		if !errors.Is(err, errMetricsServerDisabled) {
			t.Fatalf("expected errMetricsServerDisabled, got %v", err)
		}
	}

	if shutdown != nil {
		t.Fatal("expected shutdown function to be nil when server is skipped")
	}

	shutdown, err = startMetricsServer(context.Background(), zap.NewNop(), testMetricsBind, nil)
	if !errors.Is(err, errMetricsServerDisabled) {
		if err == nil {
			t.Fatal("expected errMetricsServerDisabled, got nil")
		}

		if !errors.Is(err, errMetricsServerDisabled) {
			t.Fatalf("expected errMetricsServerDisabled, got %v", err)
		}
	}

	if shutdown != nil {
		t.Fatal("expected shutdown function to be nil when handler is missing")
	}
}

func TestStartMetricsServerRequiresContext(t *testing.T) {
	t.Parallel()

	var nilContext context.Context

	shutdown, err := startMetricsServer(
		nilContext,
		zap.NewNop(),
		testMetricsBind,
		http.NewServeMux(),
	)
	if !errors.Is(err, errMetricsContextRequired) {
		t.Fatalf("expected errMetricsContextRequired, got %v", err)
	}

	if shutdown != nil {
		t.Fatal("expected shutdown function to be nil when context is missing")
	}
}

//nolint:funlen // test exercises server lifecycle and shutdown paths in one flow.
func TestStartMetricsServerServesRequests(t *testing.T) {
	t.Parallel()

	addr := freeTCPAddress(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	requestCh := make(chan struct{}, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		requestCh <- struct{}{}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	shutdown, err := startMetricsServer(ctx, nil, addr, mux)
	if err != nil {
		t.Fatalf("startMetricsServer returned error: %v", err)
	}

	if shutdown == nil {
		t.Fatal("expected shutdown function to be returned")
	}

	url := fmt.Sprintf("http://%s/metrics", addr)
	deadline := time.Now().Add(2 * time.Second)

	for {
		requestCtx, cancelRequest := context.WithTimeout(ctx, 500*time.Millisecond)

		req, reqErr := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
		if reqErr != nil {
			cancelRequest()
			t.Fatalf("build http request: %v", reqErr)
		}

		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()

			cancelRequest()

			break
		}

		cancelRequest()

		if time.Now().After(deadline) {
			t.Fatalf("failed to reach metrics endpoint: %v", err)
		}

		time.Sleep(25 * time.Millisecond)
	}

	select {
	case <-requestCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected handler to observe request")
	}

	cancel()

	shutdown(context.Background())
}

func TestStartMetricsServerFailsWhenAddressInUse(t *testing.T) {
	t.Parallel()

	var listenCfg net.ListenConfig

	listener, err := listenCfg.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	defer func() {
		_ = listener.Close()
	}()

	addr := listener.Addr().String()

	shutdown, err := startMetricsServer(
		context.Background(),
		zap.NewNop(),
		addr,
		http.NewServeMux(),
	)
	if err == nil {
		t.Fatal("expected error when address is already in use")
	}

	if !strings.Contains(err.Error(), "listen metrics endpoint") {
		t.Fatalf("expected listen error, got %v", err)
	}

	if shutdown != nil {
		t.Fatal("expected shutdown function to be nil when start fails")
	}
}

func assertMetricsEndpointStarted(
	t *testing.T,
	shutdown metricsShutdownFunc,
	cancel context.CancelFunc,
	startCalled bool,
	startAddr string,
	expectedAddr string,
	startHandler http.Handler,
	expectedHandler http.Handler,
	startLogger *zap.Logger,
) {
	t.Helper()

	if shutdown == nil {
		t.Fatal("expected shutdown function")
	}

	if cancel == nil {
		t.Fatal("expected cancel function")
	}

	if !startCalled {
		t.Fatal("expected startMetricsServer to be invoked")
	}

	if startAddr != expectedAddr {
		t.Fatalf("expected bind address %q, got %q", expectedAddr, startAddr)
	}

	if startHandler != expectedHandler {
		t.Fatalf("expected handler to be forwarded, got %v", startHandler)
	}

	if startLogger == nil {
		t.Fatal("expected logger to be forwarded")
	}
}
