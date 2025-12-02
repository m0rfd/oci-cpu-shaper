package metricsserver_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"oci-cpu-shaper/internal/metricsserver"
)

func TestStartServerRequiresContext(t *testing.T) {
	t.Parallel()

	core, _ := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(http.ResponseWriter, *http.Request) {})

	var nilCtx context.Context

	shutdown, err := metricsserver.StartServer(nilCtx, logger, "127.0.0.1:0", handler)
	if !errors.Is(err, metricsserver.ErrContextRequired) {
		t.Fatalf("expected ErrContextRequired, got %v", err)
	}

	if shutdown != nil {
		t.Fatalf("expected nil shutdown function, got %v", shutdown)
	}
}

func TestStartServerNilHandler(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	shutdown, err := metricsserver.StartServer(context.Background(), logger, "127.0.0.1:0", nil)
	if !errors.Is(err, metricsserver.ErrServerDisabled) {
		t.Fatalf("expected ErrServerDisabled, got %v", err)
	}

	if shutdown != nil {
		t.Fatalf("expected nil shutdown function, got %v", shutdown)
	}

	entries := logs.FilterMessage("metrics server disabled").All()
	if len(entries) != 1 {
		t.Fatalf("expected one disable log entry, got %d", len(entries))
	}

	if reason := entries[0].ContextMap()["reason"]; reason != "handler missing" {
		t.Fatalf("expected reason to be 'handler missing', got %v", reason)
	}
}

func TestStartServerEmptyBindAddress(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(http.ResponseWriter, *http.Request) {})

	shutdown, err := metricsserver.StartServer(context.Background(), logger, "   \t  ", handler)
	if !errors.Is(err, metricsserver.ErrServerDisabled) {
		t.Fatalf("expected ErrServerDisabled, got %v", err)
	}

	if shutdown != nil {
		t.Fatalf("expected nil shutdown function, got %v", shutdown)
	}

	entries := logs.FilterMessage("metrics server disabled").All()
	if len(entries) != 1 {
		t.Fatalf("expected one disable log entry, got %d", len(entries))
	}

	if reason := entries[0].ContextMap()["reason"]; reason != "http bind address empty" {
		t.Fatalf("expected reason to be 'http bind address empty', got %v", reason)
	}
}

func TestStartServerHappyPath(t *testing.T) {
	t.Parallel()

	var listenCfg net.ListenConfig

	listener, err := listenCfg.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("expected to acquire listener: %v", err)
	}

	addr := listener.Addr().String()

	closeErr := listener.Close()
	if closeErr != nil {
		t.Fatalf("expected listener to close: %v", closeErr)
	}

	core, _ := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	shutdown, err := metricsserver.StartServer(ctx, logger, addr, handler)
	if err != nil {
		t.Fatalf("expected server to start, got %v", err)
	}

	//nolint:exhaustruct // using default transport with custom timeout
	client := http.Client{
		Timeout: time.Second,
	}

	waitForServer(t, &client, addr)

	start := time.Now()

	cancel()

	shutdownCtx, cancelShutdown := context.WithTimeout(
		context.Background(),
		metricsserver.ShutdownTimeout,
	)
	defer cancelShutdown()

	shutdown(shutdownCtx)

	if elapsed := time.Since(start); elapsed > metricsserver.ShutdownTimeout {
		t.Fatalf(
			"expected shutdown to finish within %v, took %v",
			metricsserver.ShutdownTimeout,
			elapsed,
		)
	}

	assertServerStopped(t, &client, addr)
}

func waitForServer(t *testing.T, client *http.Client, addr string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)

	for {
		req, reqErr := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"http://"+addr,
			nil,
		)
		if reqErr != nil {
			t.Fatalf("expected request to be constructed: %v", reqErr)
		}

		resp, callErr := client.Do(req)
		if callErr == nil {
			closeErr := resp.Body.Close()
			if closeErr != nil {
				t.Fatalf("expected response body to close: %v", closeErr)
			}

			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("server did not start: %v", callErr)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func assertServerStopped(t *testing.T, client *http.Client, addr string) {
	t.Helper()

	req, reqErr := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"http://"+addr,
		nil,
	)
	if reqErr != nil {
		t.Fatalf("expected request to be constructed: %v", reqErr)
	}

	resp, callErr := client.Do(req)
	if callErr == nil {
		closeErr := resp.Body.Close()
		if closeErr != nil {
			t.Fatalf("expected response body to close: %v", closeErr)
		}

		t.Fatal("expected server to stop after shutdown")
	}
}

func TestStartEndpointMissingDeps(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(http.ResponseWriter, *http.Request) {})

	shutdown, cancel, err := metricsserver.StartEndpoint(
		context.Background(),
		metricsserver.EndpointDeps{StartServer: nil},
		logger,
		"127.0.0.1:9090",
		handler,
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if shutdown != nil || cancel != nil {
		t.Fatalf("expected endpoint startup to be skipped")
	}

	entries := logs.FilterMessage("metrics server disabled").All()
	if len(entries) != 1 {
		t.Fatalf("expected disable log entry, got %d", len(entries))
	}

	if reason := entries[0].ContextMap()["reason"]; reason != "start function missing" {
		t.Fatalf("expected reason 'start function missing', got %v", reason)
	}
}

func TestStartEndpointBlankBindAddress(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	started := false
	fakeStartServer := func(
		_ context.Context,
		_ *zap.Logger,
		_ string,
		_ http.Handler,
	) (metricsserver.ShutdownFunc, error) {
		started = true

		return func(context.Context) {}, nil
	}

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(http.ResponseWriter, *http.Request) {})

	shutdown, cancel, err := metricsserver.StartEndpoint(
		context.Background(),
		metricsserver.EndpointDeps{StartServer: fakeStartServer},
		logger,
		"\n\t ",
		handler,
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if shutdown != nil || cancel != nil {
		t.Fatalf("expected endpoint startup to be skipped")
	}

	if started {
		t.Fatal("expected StartServer not to be called for blank bind address")
	}

	entries := logs.FilterMessage("metrics server disabled").All()
	if len(entries) != 1 {
		t.Fatalf("expected disable log entry, got %d", len(entries))
	}

	if reason := entries[0].ContextMap()["reason"]; reason != "http bind address empty" {
		t.Fatalf("expected reason 'http bind address empty', got %v", reason)
	}
}
