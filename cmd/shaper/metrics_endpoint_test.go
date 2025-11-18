package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"oci-cpu-shaper/pkg/adapt"
	metricshttp "oci-cpu-shaper/pkg/http/metrics"
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

func freeTCPAddress(t *testing.T) string {
	t.Helper()

	var listenCfg net.ListenConfig

	listener, err := listenCfg.Listen(context.Background(), "tcp", testMetricsBind)
	if err != nil {
		t.Fatalf("allocate tcp port: %v", err)
	}

	addr := listener.Addr().String()

	closeErr := listener.Close()
	if closeErr != nil {
		t.Fatalf("close listener: %v", closeErr)
	}

	return addr
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

func TestConfigureMetricsSkipsExporterWhenMissing(t *testing.T) {
	t.Parallel()

	handler := configureMetrics(zap.NewNop(), nil, nil, nil, nil)
	if handler != nil {
		t.Fatal("expected handler to be nil when exporter is missing")
	}
}

func TestConfigureMetricsSetsWorkerMetrics(t *testing.T) {
	t.Parallel()

	exporter := metricshttp.NewExporter()
	pool := &stubPoolStarter{startCount: 0, workers: 3, quantum: 150 * time.Millisecond}

	_ = configureMetrics(zap.NewNop(), exporter, pool, nil, nil)

	snapshot, err := exporter.Render()
	if err != nil {
		t.Fatalf("render metrics: %v", err)
	}

	if !bytes.Contains(snapshot, []byte("worker_count 3")) {
		t.Fatalf("expected worker count metric, got %s", snapshot)
	}

	if !bytes.Contains(snapshot, []byte("duty_cycle_ms 150.000")) {
		t.Fatalf("expected duty cycle metric, got %s", snapshot)
	}
}

func TestConfigureMetricsRegistersHandlers(t *testing.T) {
	t.Parallel()

	exporter := metricshttp.NewExporter()
	pool := &stubPoolStarter{startCount: 0, workers: 5, quantum: 200 * time.Millisecond}
	controller := &stubController{
		mode:        modeDryRun,
		runErr:      nil,
		runCalled:   false,
		deadline:    time.Time{},
		deadlineSet: false,
		state:       adapt.StateFallback,
		lastErr:     errStubControllerRun,
		estErr:      errStubQueryFailure,
	}

	handler := configureMetrics(zap.NewNop(), exporter, pool, controller, nil)
	if handler == nil {
		t.Fatal("expected handler to be configured")
	}

	metricsRequest := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRecorder := httptest.NewRecorder()
	handler.ServeHTTP(metricsRecorder, metricsRequest)

	if metricsRecorder.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected metrics response 200, got %d", metricsRecorder.Result().StatusCode)
	}

	metricsBody := metricsRecorder.Body.Bytes()
	if !bytes.Contains(metricsBody, []byte("worker_count 5")) {
		t.Fatalf("expected metrics to include worker count, got %s", metricsBody)
	}

	healthRequest := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthRecorder := httptest.NewRecorder()
	handler.ServeHTTP(healthRecorder, healthRequest)

	if healthRecorder.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected health status 200, got %d", healthRecorder.Result().StatusCode)
	}

	healthBody := healthRecorder.Body.Bytes()
	if !bytes.Contains(healthBody, []byte("\"mode\":\"dry-run\"")) {
		t.Fatalf("expected controller mode in health response, got %s", healthBody)
	}

	if !bytes.Contains(healthBody, []byte("\"state\":\"fallback\"")) {
		t.Fatalf("expected fallback state in health response, got %s", healthBody)
	}

	if !bytes.Contains(healthBody, []byte(errStubControllerRun.Error())) {
		t.Fatalf("expected controller error in health response, got %s", healthBody)
	}

	if !bytes.Contains(healthBody, []byte(errStubQueryFailure.Error())) {
		t.Fatalf("expected estimator error in health response, got %s", healthBody)
	}
}

func TestConfigureMetricsWithoutController(t *testing.T) {
	t.Parallel()

	exporter := metricshttp.NewExporter()

	handler := configureMetrics(zap.NewNop(), exporter, nil, nil, nil)
	if handler == nil {
		t.Fatal("expected handler to be configured")
	}

	healthRequest := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, healthRequest)

	if recorder.Result().StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing health handler, got %d", recorder.Result().StatusCode)
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

func configureMetricsHandlerForTest(
	t *testing.T,
	exporter *metricshttp.Exporter,
	pool poolStarter,
	controller adapt.Controller,
) http.Handler {
	t.Helper()

	handler := configureMetrics(zap.NewNop(), exporter, pool, controller, nil)
	if handler == nil {
		t.Fatal("expected handler to be configured")
	}

	return handler
}

func TestConfigureMetricsServesPrometheusText(t *testing.T) {
	t.Parallel()

	exporter := metricshttp.NewExporter()
	exporter.SetMode("enforce")
	exporter.SetState("normal")
	exporter.SetTarget(0.42)
	exporter.ObserveOCIP95(0.31, time.Unix(1_700_000_333, 0))
	exporter.ObserveHostCPU(0.55)

	pool := &stubPoolStarter{startCount: 0, workers: 3, quantum: 2 * time.Millisecond}
	controller := &stubController{
		mode:        "enforce",
		runErr:      nil,
		runCalled:   false,
		deadline:    time.Time{},
		deadlineSet: false,
		state:       adapt.StateNormal,
		lastErr:     nil,
		estErr:      nil,
	}

	handler := configureMetricsHandlerForTest(t, exporter, pool, controller)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", recorder.Code)
	}

	const promContentType = "application/openmetrics-text; version=1.0.0; charset=utf-8"
	if got := recorder.Header().Get("Content-Type"); got != promContentType {
		t.Fatalf("expected Prometheus content type, got %q", got)
	}

	body := recorder.Body.String()
	for _, snippet := range []string{
		"# HELP shaper_target_ratio",
		`shaper_mode{mode="enforce"} 1`,
		"shaper_enforcing 1",
		"worker_count 3",
		"duty_cycle_ms 2.000",
		"oci_last_success_epoch 1700000333",
	} {
		if !strings.Contains(body, snippet) {
			t.Fatalf("expected metrics output to contain %q, got:\n%s", snippet, body)
		}
	}
}
