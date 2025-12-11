package metricsserver_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"oci-cpu-shaper/internal/metricsserver"
)

var (
	errListenShouldNotBeCalled = errors.New("listener should not be called")
	errListenBoom              = errors.New("boom")
)

func newTestLogger(level zapcore.Level) (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(level)

	return zap.New(core), logs
}

//nolint:paralleltest // modifies global listener stub
func TestStartServerDisabledSkipsListener(
	t *testing.T,
) {
	stubListenEndpoint(t, func(context.Context, string, string) (net.Listener, error) {
		t.Fatal("listener should not be called when server is disabled")

		return nil, errListenShouldNotBeCalled
	})

	logger, logs := newTestLogger(zap.InfoLevel)

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(http.ResponseWriter, *http.Request) {})

	shutdown, err := metricsserver.StartServer(context.Background(), logger, "  \t \n", handler)
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
		t.Fatalf("expected reason 'http bind address empty', got %v", reason)
	}
}

//nolint:paralleltest // modifies global listener stub
func TestStartServerNilContextSkipsListener(
	t *testing.T,
) {
	stubListenEndpoint(t, func(context.Context, string, string) (net.Listener, error) {
		t.Fatal("listener should not be called when context is nil")

		return nil, errListenShouldNotBeCalled
	})

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(http.ResponseWriter, *http.Request) {})

	shutdown, err := metricsserver.StartServer(nil, zap.NewNop(), "127.0.0.1:0", handler)
	if !errors.Is(err, metricsserver.ErrContextRequired) {
		t.Fatalf("expected ErrContextRequired, got %v", err)
	}

	if shutdown != nil {
		t.Fatalf("expected nil shutdown function, got %v", shutdown)
	}
}

//nolint:paralleltest // modifies global listener stub
func TestStartServerListenErrorLogs(
	t *testing.T,
) {
	listenErr := errListenBoom

	stubListenEndpoint(t, func(_ context.Context, network, address string) (net.Listener, error) {
		if network != "tcp" {
			t.Fatalf("expected tcp network, got %q", network)
		}

		if address != "127.0.0.1:0" {
			t.Fatalf("unexpected address: %q", address)
		}

		return nil, listenErr
	})

	logger, logs := newTestLogger(zap.ErrorLevel)

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(http.ResponseWriter, *http.Request) {})

	shutdown, err := metricsserver.StartServer(context.Background(), logger, "127.0.0.1:0", handler)
	if !errors.Is(err, listenErr) {
		t.Fatalf("expected listen error to be wrapped, got %v", err)
	}

	if shutdown != nil {
		t.Fatalf("expected nil shutdown function, got %v", shutdown)
	}

	entries := logs.FilterMessage("metrics server listen failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected one listen log entry, got %d", len(entries))
	}

	log := entries[0]

	if bind := log.ContextMap()["bind"]; bind != "127.0.0.1:0" {
		t.Fatalf("expected bind address 127.0.0.1:0, got %v", bind)
	}

	if log.Entry.Level != zap.ErrorLevel {
		t.Fatalf("expected error level log, got %v", log.Entry.Level)
	}

	errField, ok := log.ContextMap()["error"].(string)
	if !ok {
		t.Fatalf("expected error field in log context, got %v", log.ContextMap()["error"])
	}

	if errField != listenErr.Error() {
		t.Fatalf("expected log error %q, got %q", listenErr.Error(), errField)
	}
}

func stubListenEndpoint(
	t *testing.T,
	stub func(context.Context, string, string) (net.Listener, error),
) {
	t.Helper()

	restore := metricsserver.SetListenEndpointForTest(stub)

	t.Cleanup(restore)
}
