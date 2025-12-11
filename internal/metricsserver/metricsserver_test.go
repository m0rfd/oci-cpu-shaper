package metricsserver_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"oci-cpu-shaper/internal/metricsserver"
)

const (
	bindAddrDefault            = "127.0.0.1:0"
	reasonHTTPBindAddressEmpty = "http bind address empty"
)

var listenEndpointMu sync.Mutex //nolint:gochecknoglobals // protects listenEndpoint test stub

var (
	errUnexpectedListen = errors.New("unexpected listen")
	errListenFailure    = errors.New("listen failure")
)

type testAddr string

func (addr testAddr) Network() string {
	return "tcp"
}

func (addr testAddr) String() string {
	return string(addr)
}

type waitingListener struct {
	addr         net.Addr
	closed       chan struct{}
	closeOnce    sync.Once
	acceptOnce   sync.Once
	acceptCalled chan struct{}
}

func newWaitingListener(addr string) *waitingListener {
	return &waitingListener{
		addr:         testAddr(addr),
		closed:       make(chan struct{}),
		closeOnce:    sync.Once{},
		acceptOnce:   sync.Once{},
		acceptCalled: make(chan struct{}),
	}
}

func (listener *waitingListener) Accept() (net.Conn, error) {
	listener.acceptOnce.Do(func() {
		close(listener.acceptCalled)
	})

	<-listener.closed

	return nil, http.ErrServerClosed
}

func (listener *waitingListener) Close() error {
	listener.closeOnce.Do(func() {
		close(listener.closed)
	})

	return nil
}

func (listener *waitingListener) Addr() net.Addr {
	return listener.addr
}

func TestStartServerNilContext(t *testing.T) {
	t.Parallel()

	called := false

	stubListenEndpoint(t, func(context.Context, string, string) (net.Listener, error) {
		called = true

		return nil, errUnexpectedListen
	})

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(http.ResponseWriter, *http.Request) {})

	shutdown, err := metricsserver.StartServer(nil, zap.NewNop(), bindAddrDefault, handler)
	if !errors.Is(err, metricsserver.ErrContextRequired) {
		t.Fatalf("expected ErrContextRequired, got %v", err)
	}

	if shutdown != nil {
		t.Fatalf("expected nil shutdown function, got %v", shutdown)
	}

	if called {
		t.Fatal("expected listen endpoint not to be called without context")
	}
}

func TestStartServerNilHandler(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	called := false

	stubListenEndpoint(t, func(context.Context, string, string) (net.Listener, error) {
		called = true

		return nil, errUnexpectedListen
	})

	shutdown, err := metricsserver.StartServer(context.Background(), logger, bindAddrDefault, nil)
	if !errors.Is(err, metricsserver.ErrServerDisabled) {
		t.Fatalf("expected ErrServerDisabled, got %v", err)
	}

	if shutdown != nil {
		t.Fatalf("expected nil shutdown function, got %v", shutdown)
	}

	if called {
		t.Fatal("expected listen endpoint not to be called when handler is nil")
	}

	entries := logs.FilterMessage("metrics server disabled").All()
	if len(entries) != 1 {
		t.Fatalf("expected one disable log entry, got %d", len(entries))
	}

	if entries[0].Entry.Level != zap.WarnLevel {
		t.Fatalf("expected warning log level, got %v", entries[0].Entry.Level)
	}

	if reason := entries[0].ContextMap()["reason"]; reason != "handler missing" {
		t.Fatalf("expected reason to be 'handler missing', got %v", reason)
	}
}

func TestStartServerEmptyBindAddress(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	called := false

	stubListenEndpoint(t, func(context.Context, string, string) (net.Listener, error) {
		called = true

		return nil, errUnexpectedListen
	})

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(http.ResponseWriter, *http.Request) {})

	shutdown, err := metricsserver.StartServer(context.Background(), logger, "   \t  ", handler)
	if !errors.Is(err, metricsserver.ErrServerDisabled) {
		t.Fatalf("expected ErrServerDisabled, got %v", err)
	}

	if shutdown != nil {
		t.Fatalf("expected nil shutdown function, got %v", shutdown)
	}

	if called {
		t.Fatal("expected listen endpoint not to be called for empty address")
	}

	entries := logs.FilterMessage("metrics server disabled").All()
	if len(entries) != 1 {
		t.Fatalf("expected one disable log entry, got %d", len(entries))
	}

	if reason := entries[0].ContextMap()["reason"]; reason != reasonHTTPBindAddressEmpty {
		t.Fatalf("expected reason to be 'http bind address empty', got %v", reason)
	}
}

func TestStartServerListenFailure(t *testing.T) {
	t.Parallel()

	stubListenEndpoint(t, func(_ context.Context, _, _ string) (net.Listener, error) {
		return nil, errListenFailure
	})

	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(http.ResponseWriter, *http.Request) {})

	shutdown, err := metricsserver.StartServer(
		context.Background(),
		logger,
		"127.0.0.1:9090",
		handler,
	)
	if err == nil {
		t.Fatal("expected listen error, got nil")
	}

	if !strings.Contains(err.Error(), "listen metrics endpoint") {
		t.Fatalf("expected listen error to be wrapped, got %v", err)
	}

	if shutdown != nil {
		t.Fatalf("expected nil shutdown function on listen failure, got %v", shutdown)
	}

	entries := logs.FilterMessage("metrics server listen failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected one listen failure log entry, got %d", len(entries))
	}

	if entries[0].ContextMap()["bind"] != "127.0.0.1:9090" {
		t.Fatalf("expected bind address to be logged, got %v", entries[0].ContextMap()["bind"])
	}
}

func TestStartServerSuccessfulShutdownRespectsTimeout(t *testing.T) {
	t.Parallel()

	listener := newWaitingListener(bindAddrDefault)

	stubListenEndpoint(t, func(_ context.Context, _, addr string) (net.Listener, error) {
		if addr != bindAddrDefault {
			t.Fatalf("expected bind address %q, got %q", bindAddrDefault, addr)
		}

		return listener, nil
	},
	)

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(http.ResponseWriter, *http.Request) {})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	shutdown, err := metricsserver.StartServer(ctx, logger, bindAddrDefault, handler)
	if err != nil {
		t.Fatalf("expected server to start, got %v", err)
	}

	select {
	case <-listener.acceptCalled:
	case <-time.After(time.Second):
		t.Fatal("expected serve goroutine to begin accepting")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelShutdown()

	start := time.Now()

	shutdown(shutdownCtx)

	duration := time.Since(start)

	if duration > 200*time.Millisecond {
		t.Fatalf("expected shutdown to respect timeout, took %v", duration)
	}

	entries := logs.FilterMessage("metrics server stopped").All()
	if len(entries) != 1 {
		t.Fatalf("expected server stopped log entry after shutdown, got %d", len(entries))
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

func TestStartEndpointNilContext(t *testing.T) {
	t.Parallel()

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(http.ResponseWriter, *http.Request) {})

	shutdown, cancel, err := metricsserver.StartEndpoint(
		nil,
		metricsserver.EndpointDeps{StartServer: metricsserver.StartServer},
		zap.NewNop(),
		"127.0.0.1:8080",
		handler,
	)
	if !errors.Is(err, metricsserver.ErrContextRequired) {
		t.Fatalf("expected ErrContextRequired, got %v", err)
	}

	if shutdown != nil || cancel != nil {
		t.Fatalf("expected shutdown and cancel to be nil when context missing")
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

	if reason := entries[0].ContextMap()["reason"]; reason != reasonHTTPBindAddressEmpty {
		t.Fatalf("expected reason 'http bind address empty', got %v", reason)
	}
}

func TestStartEndpointNilHandler(t *testing.T) {
	t.Parallel()

	called := false
	fakeStart := func(_ context.Context, _ *zap.Logger, _ string, _ http.Handler) (metricsserver.ShutdownFunc, error) {
		called = true

		return func(context.Context) {}, nil
	}

	shutdown, cancel, err := metricsserver.StartEndpoint(
		context.Background(),
		metricsserver.EndpointDeps{StartServer: fakeStart},
		zap.NewNop(),
		bindAddrDefault,
		nil,
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if shutdown != nil || cancel != nil {
		t.Fatalf("expected nil shutdown and cancel when handler is nil")
	}

	if called {
		t.Fatal("expected start server not to be called when handler is nil")
	}
}

func TestStartEndpointCancelShutsDownServer(t *testing.T) {
	t.Parallel()

	canceled := make(chan struct{})

	fakeStart := func(
		ctx context.Context,
		_ *zap.Logger,
		addr string,
		handler http.Handler,
	) (metricsserver.ShutdownFunc, error) {
		if addr != bindAddrDefault {
			t.Fatalf("unexpected bind address: %q", addr)
		}

		if handler == nil {
			t.Fatal("expected handler to be passed to start function")
		}

		var once sync.Once

		go func() {
			<-ctx.Done()
			once.Do(func() {
				close(canceled)
			})
		}()

		return func(context.Context) {
			once.Do(func() {
				close(canceled)
			})
		}, nil
	}

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(http.ResponseWriter, *http.Request) {})

	shutdown, cancel, err := metricsserver.StartEndpoint(
		context.Background(),
		metricsserver.EndpointDeps{StartServer: fakeStart},
		zap.NewNop(),
		bindAddrDefault,
		handler,
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if shutdown == nil || cancel == nil {
		t.Fatalf("expected shutdown and cancel to be returned")
	}

	cancel()
	shutdown(nil)

	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("expected server to cancel when context is canceled")
	}
}
