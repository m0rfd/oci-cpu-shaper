//go:build integration

package integration

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"oci-cpu-shaper/internal/metricsserver"
)

func TestMetricsEndpointSkipsAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("handler nil", func(t *testing.T) {
		t.Parallel()

		shutdown, cancel, err := metricsserver.StartEndpoint(
			context.Background(),
			metricsserver.EndpointDeps{StartServer: metricsserver.StartServer},
			zap.NewNop(),
			freeTCPAddr(t),
			nil,
		)
		if err != nil {
			t.Fatalf("start metrics endpoint: %v", err)
		}

		if shutdown != nil || cancel != nil {
			t.Fatalf("expected nil shutdown and cancel when handler is nil")
		}
	})

	t.Run("start function missing", func(t *testing.T) {
		t.Parallel()

		core, observed := observer.New(zap.InfoLevel)
		logger := zap.New(core)

		shutdown, cancel, err := metricsserver.StartEndpoint(
			context.Background(),
			metricsserver.EndpointDeps{},
			logger,
			freeTCPAddr(t),
			http.NewServeMux(),
		)
		if err != nil {
			t.Fatalf("start metrics endpoint: %v", err)
		}

		if shutdown != nil || cancel != nil {
			t.Fatalf("expected nil shutdown and cancel when starter is missing")
		}

		entries := observed.FilterMessage("metrics server disabled").All()
		if len(entries) != 1 {
			t.Fatalf("expected single disabled log entry, got %d", len(entries))
		}

		requireLogField(t, entries[0], "reason", "start function missing")
	})

	t.Run("empty bind short-circuits", func(t *testing.T) {
		t.Parallel()

		called := make(chan struct{}, 1)

		shutdown, cancel, err := metricsserver.StartEndpoint(
			context.Background(),
			metricsserver.EndpointDeps{StartServer: func(ctx context.Context, logger *zap.Logger, addr string, handler http.Handler) (metricsserver.ShutdownFunc, error) {
				select {
				case called <- struct{}{}:
				default:
				}

				return metricsserver.StartServer(ctx, logger, addr, handler)
			}},
			zap.NewNop(),
			"   ",
			http.NewServeMux(),
		)
		if err != nil {
			t.Fatalf("start metrics endpoint: %v", err)
		}

		if shutdown != nil || cancel != nil {
			t.Fatalf("expected nil shutdown and cancel when bind address is empty")
		}

		select {
		case <-called:
			t.Fatalf("metrics server starter should not be invoked when bind is empty")
		default:
		}
	})

	t.Run("nil context surfaces error", func(t *testing.T) {
		t.Parallel()

		shutdown, cancel, err := metricsserver.StartEndpoint(
			nil,
			metricsserver.EndpointDeps{StartServer: metricsserver.StartServer},
			zap.NewNop(),
			freeTCPAddr(t),
			http.NewServeMux(),
		)
		if !errors.Is(err, metricsserver.ErrContextRequired) {
			t.Fatalf("expected errMetricsContextRequired, got %v", err)
		}

		if shutdown != nil || cancel != nil {
			t.Fatalf("expected nil shutdown and cancel when context is nil")
		}
	})
}

//nolint:paralleltest // lifecycle assertions coordinate shared goroutines.
func TestMetricsServerShutdownWaitsForServeCompletion(t *testing.T) {
	addr := freeTCPAddr(t)

	startCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	requestStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	handler := http.NewServeMux()
	handler.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		select {
		case requestStarted <- struct{}{}:
		default:
		}

		<-release

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	shutdown, err := metricsserver.StartServer(startCtx, logger, addr, handler)
	if err != nil {
		t.Fatalf("start metrics server: %v", err)
	}

	client := http.Client{ //nolint:exhaustruct // timeout configured
		Timeout: time.Second,
	}
	reqCtx, reqCancel := context.WithTimeout(context.Background(), time.Second)
	defer reqCancel()

	requestDone := make(chan error, 1)
	go func() {
		req, reqErr := http.NewRequestWithContext(reqCtx, http.MethodGet, "http://"+addr+"/metrics", http.NoBody)
		if reqErr != nil {
			requestDone <- reqErr

			return
		}

		resp, err := client.Do(req)
		if err != nil {
			requestDone <- err

			return
		}

		_, _ = io.ReadAll(resp.Body)

		requestDone <- resp.Body.Close()
	}()

	select {
	case <-requestStarted:
	case err := <-requestDone:
		t.Fatalf("request failed before reaching handler: %v", err)
	case <-time.After(time.Second):
		t.Fatal("handler did not receive request")
	}

	cancel()

	shutdownDone := make(chan struct{})
	go func() {
		shutdown(context.Background())

		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
		t.Fatal("shutdown returned before handler completed")
	case <-time.After(200 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-requestDone:
		if err != nil {
			t.Fatalf("metrics request failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("metrics request did not complete")
	}

	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not wait for serve completion")
	}

	entries := observed.TakeAll()
	if !containsMessage(entries, "metrics server listening") {
		t.Fatal("expected metrics server to log listening state")
	}

	if !containsMessage(entries, "stopping metrics server") {
		t.Fatal("expected shutdown log entry before completion")
	}

	if !containsMessage(entries, "metrics server stopped") {
		t.Fatal("expected metrics server to log stop after shutdown")
	}
}

func freeTCPAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}

	addr := listener.Addr().String()

	if closeErr := listener.Close(); closeErr != nil {
		t.Fatalf("close listener: %v", closeErr)
	}

	return addr
}

func requireLogField(t *testing.T, entry observer.LoggedEntry, key, expected string) {
	t.Helper()

	for _, field := range entry.Context {
		if field.Key == key {
			if field.String != expected {
				t.Fatalf("expected %s=%q, got %q", key, expected, field.String)
			}

			return
		}
	}

	t.Fatalf("expected log field %q not found", key)
}

func containsMessage(entries []observer.LoggedEntry, message string) bool {
	for _, entry := range entries {
		if entry.Message == message {
			return true
		}
	}

	return false
}
