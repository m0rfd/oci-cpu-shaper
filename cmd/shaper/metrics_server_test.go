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
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const testMetricsBind = "127.0.0.1:0"

var errMetricsStartFailure = errors.New("metrics start failure")

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

//nolint:funlen // table-driven coverage of early-return paths.
func TestStartMetricsEndpointDisablesMetricsWithoutHandlersOrStarter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		deps            runDeps
		bind            string
		handler         http.Handler
		expectedLogs    int
		expectedMessage string
		expectedReason  string
	}{
		{
			name: "handler missing",
			deps: func() runDeps {
				deps := defaultRunDeps()
				deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
					t.Fatal("expected startMetricsServer not to be called when handler is nil")

					return nil, errMetricsServerBoom
				}

				return deps
			}(),
			bind:            testMetricsBind,
			handler:         nil,
			expectedLogs:    0,
			expectedMessage: "",
			expectedReason:  "",
		},
		{
			name: "starter missing",
			deps: func() runDeps {
				deps := defaultRunDeps()
				deps.startMetricsServer = nil

				return deps
			}(),
			bind: testMetricsBind,
			handler: func() http.Handler {
				mux := http.NewServeMux()
				mux.Handle("/metrics", http.NotFoundHandler())

				return mux
			}(),
			expectedLogs:    1,
			expectedMessage: "metrics server disabled",
			expectedReason:  "start function missing",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			core, observed := observer.New(zap.InfoLevel)
			logger := zap.New(core)

			shutdown, cancel, err := startMetricsEndpoint(
				context.Background(),
				testCase.deps,
				logger,
				testCase.bind,
				testCase.handler,
			)
			if err != nil {
				t.Fatalf("startMetricsEndpoint returned error: %v", err)
			}

			if shutdown != nil || cancel != nil {
				t.Fatalf("expected nil shutdown and cancel, got %v, %v", shutdown, cancel)
			}

			entries := observed.All()
			if len(entries) != testCase.expectedLogs {
				t.Fatalf("expected %d log entries, got %d", testCase.expectedLogs, len(entries))
			}

			if testCase.expectedLogs == 0 {
				return
			}

			if entries[0].Message != testCase.expectedMessage {
				t.Fatalf(
					"expected log message %q, got %q",
					testCase.expectedMessage,
					entries[0].Message,
				)
			}

			requireLogFieldString(t, entries[0], "reason", testCase.expectedReason)
		})
	}
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

//nolint:funlen // table-driven log validation across disabled paths.
func TestStartMetricsServerSkipsWhenAddressOrHandlerMissing(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		ctxFactory      func() context.Context
		addr            string
		handler         http.Handler
		expectedLevel   zapcore.Level
		expectedMessage string
		expectedReason  string
		expectedErr     error
	}{
		{
			name:            "address empty",
			ctxFactory:      context.Background,
			addr:            "   ",
			handler:         http.NewServeMux(),
			expectedLevel:   zapcore.InfoLevel,
			expectedMessage: "metrics server disabled",
			expectedReason:  "http bind address empty",
			expectedErr:     errMetricsServerDisabled,
		},
		{
			name:            "handler missing",
			ctxFactory:      context.Background,
			addr:            testMetricsBind,
			handler:         nil,
			expectedLevel:   zapcore.WarnLevel,
			expectedMessage: "metrics server disabled",
			expectedReason:  "handler missing",
			expectedErr:     errMetricsServerDisabled,
		},
		{
			name:            "context missing",
			ctxFactory:      func() context.Context { return nil },
			addr:            testMetricsBind,
			handler:         http.NewServeMux(),
			expectedLevel:   zapcore.InfoLevel,
			expectedMessage: "",
			expectedReason:  "",
			expectedErr:     errMetricsContextRequired,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			core, observed := observer.New(zapcore.InfoLevel)
			logger := zap.New(core)

			ctx := testCase.ctxFactory()

			shutdown, err := startMetricsServer(ctx, logger, testCase.addr, testCase.handler)
			if !errors.Is(err, testCase.expectedErr) {
				t.Fatalf("expected %v, got %v", testCase.expectedErr, err)
			}

			if shutdown != nil {
				t.Fatal("expected shutdown function to be nil when server is skipped")
			}

			entries := observed.TakeAll()
			if testCase.expectedMessage == "" {
				if len(entries) != 0 {
					t.Fatalf("expected no log entries, got %+v", entries)
				}

				return
			}

			if len(entries) != 1 {
				t.Fatalf("expected single log entry, got %+v", entries)
			}

			entry := entries[0]
			if entry.Message != testCase.expectedMessage {
				t.Fatalf("expected log message %q, got %q", testCase.expectedMessage, entry.Message)
			}

			if entry.Level != testCase.expectedLevel {
				t.Fatalf("expected log level %s, got %s", testCase.expectedLevel, entry.Level)
			}

			requireLogFieldString(t, entry, "reason", testCase.expectedReason)
		})
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

func TestStartMetricsEndpointWrapsStartError(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		return nil, errMetricsStartFailure
	}

	shutdown, cancel, err := startMetricsEndpoint(
		context.Background(),
		deps,
		zap.NewNop(),
		testMetricsBind,
		http.NewServeMux(),
	)

	if shutdown != nil || cancel != nil {
		t.Fatalf(
			"expected shutdown and cancel to be nil on start failure, got %v and %v",
			shutdown,
			cancel,
		)
	}

	if !errors.Is(err, errMetricsStartFailure) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
}

func TestStartMetricsEndpointWrapsContextError(t *testing.T) {
	t.Parallel()

	var nilContext context.Context

	shutdown, cancel, err := startMetricsEndpoint(
		nilContext,
		defaultRunDeps(),
		zap.NewNop(),
		testMetricsBind,
		http.NewServeMux(),
	)

	if !errors.Is(err, errMetricsContextRequired) {
		t.Fatalf("expected context error, got %v", err)
	}

	if shutdown != nil || cancel != nil {
		t.Fatalf(
			"expected nil shutdown and cancel on context error, got %v and %v",
			shutdown,
			cancel,
		)
	}
}

func TestStartMetricsEndpointDelegates(t *testing.T) {
	t.Parallel()

	startCalled := false

	deps := defaultRunDeps()
	deps.startMetricsServer = func(context.Context, *zap.Logger, string, http.Handler) (metricsShutdownFunc, error) {
		startCalled = true

		return func(context.Context) {}, nil
	}

	shutdown, cancel, err := startMetricsEndpoint(
		context.Background(),
		deps,
		zap.NewNop(),
		testMetricsBind,
		http.NewServeMux(),
	)
	if err != nil {
		t.Fatalf("start metrics endpoint: %v", err)
	}

	if !startCalled {
		t.Fatal("expected start function to be invoked")
	}

	if shutdown == nil {
		t.Fatal("expected shutdown function from delegate")
	}

	if cancel == nil {
		t.Fatal("expected cancel function from delegate")
	}

	cancel()
	shutdown(context.Background())
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

//nolint:cyclop,funlen // lifecycle assertions exercise multiple branches.
func TestStartMetricsServerShutsDownOnCancel(t *testing.T) {
	t.Parallel()

	addr := freeTCPAddress(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	core, observed := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

	received := make(chan struct{}, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		received <- struct{}{}

		w.WriteHeader(http.StatusOK)
	})

	shutdown, err := startMetricsServer(ctx, logger, addr, mux)
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
	case <-received:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected handler to observe request")
	}

	cancel()
	shutdown(context.Background())

	requestCtx, cancelRequest := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancelRequest()

	req, reqErr := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
	if reqErr != nil {
		t.Fatalf("build http request: %v", reqErr)
	}

	resp, err := http.DefaultClient.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}

	if err == nil {
		t.Fatal("expected request to fail after server shutdown")
	}

	entries := observed.All()

	var stoppingLogged, stoppedLogged bool

	for i := range entries {
		switch entries[i].Message {
		case "stopping metrics server":
			stoppingLogged = true
		case "metrics server stopped":
			stoppedLogged = true
		}
	}

	if !stoppingLogged {
		t.Fatalf("expected stopping log entry, got %+v", entries)
	}

	if !stoppedLogged {
		t.Fatalf("expected stopped log entry, got %+v", entries)
	}
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

func TestStartMetricsServerWrapsError(t *testing.T) {
	t.Parallel()

	var nilContext context.Context

	shutdown, err := startMetricsServer(
		nilContext,
		zap.NewNop(),
		testMetricsBind,
		http.NewServeMux(),
	)
	if !errors.Is(err, errMetricsContextRequired) {
		t.Fatalf("expected wrapped context error, got %v", err)
	}

	if shutdown != nil {
		t.Fatal("expected shutdown function to be nil on wrapped error")
	}
}
