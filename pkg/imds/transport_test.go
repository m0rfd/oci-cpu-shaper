package imds_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/imds"
)

var (
	errDialFailure         = errors.New("dial failure")
	errCloseBoom           = errors.New("close boom")
	errCloseFailed         = errors.New("close failure")
	errUnexpectedRoundTrip = errors.New("unexpected round trip")
)

func TestHTTPClientRetriesOnServerError(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	server := newIPv4TestServer(
		t,
		http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
			if req.URL.Path != regionResourcePath {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}

			requireIMDSAuthHeader(t, req)

			if calls.Add(1) == 1 {
				writer.WriteHeader(http.StatusInternalServerError)

				return
			}

			_, _ = writer.Write([]byte("us-ashburn-1"))
		}),
	)
	t.Cleanup(server.Close)

	httpClient := server.Client()
	httpClient.Timeout = time.Second

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL(server.URL+"/opc/v2"),
		imds.WithMaxAttempts(3),
		imds.WithBackoff(10*time.Millisecond),
	)

	ctx := context.Background()

	gotRegion, err := client.Region(ctx)
	requireNoError(t, err, "Region()")
	requireEqual(t, "Region()", gotRegion, "us-ashburn-1")
	requireEqual(t, "attempts", calls.Load(), int32(2))
}

func TestHTTPClientRetriesOnTransportError(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != regionResourcePath {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}

		requireIMDSAuthHeader(t, req)

		switch attempts.Add(1) {
		case 1:
			return nil, errDialFailure
		default:
			return newHTTPResponse(
				http.StatusOK,
				io.NopCloser(strings.NewReader("us-sanjose-1")),
				req,
			), nil
		}
	}))

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL("http://metadata.local/opc/v2"),
		imds.WithBackoff(5*time.Millisecond),
	)

	ctx := context.Background()

	gotRegion, err := client.Region(ctx)
	requireNoError(t, err, "Region()")
	requireEqual(t, "attempts", attempts.Load(), int32(2))
	requireEqual(t, "Region()", gotRegion, "us-sanjose-1")
}

func TestHTTPClientFailsToBuildRequest(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts.Add(1)

		t.Fatalf("unexpected RoundTrip for %s", req.URL)

		return nil, errUnexpectedRoundTrip
	}))

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL(":// bad url"),
		imds.WithMaxAttempts(3),
	)

	_, err := client.Region(context.Background())
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	if !strings.Contains(err.Error(), "build request for region") {
		t.Fatalf("Region() error = %v, want request build failure", err)
	}

	requireEqual(t, "attempts", attempts.Load(), int32(0))
}

func TestHTTPClientRetryableStatusExhaustsBudgetWithFakeClient(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		attempts.Add(1)

		return newHTTPResponse(
			http.StatusServiceUnavailable,
			io.NopCloser(strings.NewReader("retry later")),
			req,
		), nil
	}))

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL("http://metadata.local/opc/v2"),
		imds.WithMaxAttempts(3),
		imds.WithBackoff(5*time.Millisecond),
	)

	_, err := client.Region(context.Background())
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	if !strings.Contains(err.Error(), "exhausted retry budget") {
		t.Fatalf("Region() error = %v, want exhausted retry budget", err)
	}

	if !strings.Contains(err.Error(), "retryable status code") {
		t.Fatalf("Region() error = %v, want retryable status", err)
	}

	requireEqual(t, "attempts", attempts.Load(), int32(3))
}

func TestHTTPClientContextCanceledDuringRequest(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != regionResourcePath {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}

		requireIMDSAuthHeader(t, req)

		attempts.Add(1)

		cancelRaw := req.Context().Value(cancelFuncKey{})

		cancel, ok := cancelRaw.(context.CancelFunc)
		if !ok {
			t.Fatalf("missing cancel func in context: %T", cancelRaw)
		}

		cancel()

		return nil, context.Canceled
	}))

	ctx, cancel := context.WithCancel(context.Background())
	ctx = context.WithValue(ctx, cancelFuncKey{}, cancel)

	client := imds.NewClient(httpClient, imds.WithBaseURL("http://metadata.local/opc/v2"))

	_, err := client.Region(ctx)
	if err == nil {
		t.Fatalf("Region() expected error, got nil")
	}

	if !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("Region() error = %v, want context canceled", err)
	}

	requireEqual(t, "attempts", attempts.Load(), int32(1))
}

func TestHTTPClientNetworkErrorWithContextCancellationSkipsRetry(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		attempts.Add(1)

		cancelRaw := req.Context().Value(cancelFuncKey{})

		cancel, ok := cancelRaw.(context.CancelFunc)
		if !ok {
			t.Fatalf("missing cancel func in context: %T", cancelRaw)
		}

		cancel()

		return nil, context.Canceled
	}))

	ctx, cancel := context.WithCancel(context.Background())
	ctx = context.WithValue(ctx, cancelFuncKey{}, cancel)
	t.Cleanup(cancel)

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL("http://metadata.local/opc/v2"),
		imds.WithMaxAttempts(2),
		imds.WithBackoff(5*time.Millisecond),
	)

	_, err := client.Region(ctx)
	if err == nil {
		t.Fatalf("Region() expected error, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Region() error = %v, want context canceled", err)
	}

	if !strings.Contains(err.Error(), "imds: request execution failed") {
		t.Fatalf("Region() error = %v, want wrapped execution failure", err)
	}

	requireEqual(t, "attempts", attempts.Load(), int32(1))
}

func TestHTTPClientNetworkErrorRetriesWithoutContextCancellation(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		attempts.Add(1)

		return nil, errDialFailure
	}))

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL("http://metadata.local/opc/v2"),
		imds.WithMaxAttempts(2),
		imds.WithBackoff(5*time.Millisecond),
	)

	_, err := client.Region(context.Background())
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	if !strings.Contains(err.Error(), "imds: exhausted retry budget") {
		t.Fatalf("Region() error = %v, want exhausted retry budget", err)
	}

	if !strings.Contains(err.Error(), errDialFailure.Error()) {
		t.Fatalf("Region() error = %v, want wrapped network failure", err)
	}

	requireEqual(t, "attempts", attempts.Load(), int32(2))
}

func TestHTTPClientContextCanceledBeforeCallWrapsError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var attempts atomic.Int32

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		attempts.Add(1)

		return nil, errUnexpectedRoundTrip
	}))

	client := imds.NewClient(httpClient, imds.WithBaseURL("http://metadata.local/opc/v2"))

	_, err := client.Region(ctx)
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	if !strings.Contains(err.Error(), "imds: request execution failed") {
		t.Fatalf("Region() error = %v, want wrapped execution error", err)
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Region() error = %v, want context canceled", err)
	}

	requireEqual(t, "attempts", attempts.Load(), int32(1))
}

func TestHTTPClientWaitsBetweenRetryableAttempts(t *testing.T) {
	t.Parallel()

	const backoff = 50 * time.Millisecond

	var (
		attempts      atomic.Int32
		firstAttempt  time.Time
		secondAttempt time.Time
	)

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		switch attempts.Add(1) {
		case 1:
			firstAttempt = time.Now()

			return newHTTPResponse(
				http.StatusServiceUnavailable,
				io.NopCloser(strings.NewReader("retryable")),
				req,
			), nil
		case 2:
			secondAttempt = time.Now()

			return newHTTPResponse(
				http.StatusOK,
				io.NopCloser(strings.NewReader("us-phoenix-1")),
				req,
			), nil
		default:
			t.Fatalf("unexpected attempt: %d", attempts.Load())

			return nil, errUnexpectedRoundTrip
		}
	}))

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL("http://metadata.local/opc/v2"),
		imds.WithMaxAttempts(2),
		imds.WithBackoff(backoff),
	)

	region, err := client.Region(context.Background())
	requireNoError(t, err, "Region()")
	requireEqual(t, "Region()", region, "us-phoenix-1")

	if secondAttempt.Sub(firstAttempt) < backoff {
		t.Fatalf("retry waited %v, want at least %v", secondAttempt.Sub(firstAttempt), backoff)
	}

	requireEqual(t, "attempts", attempts.Load(), int32(2))
}

func TestHTTPClientNonRetryableStatusSkipsBackoff(t *testing.T) {
	t.Parallel()

	const backoff = 200 * time.Millisecond

	var attempts atomic.Int32

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		attempts.Add(1)

		return newHTTPResponse(
			http.StatusBadRequest,
			io.NopCloser(strings.NewReader("bad request")),
			req,
		), nil
	}))

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL("http://metadata.local/opc/v2"),
		imds.WithMaxAttempts(3),
		imds.WithBackoff(backoff),
	)

	start := time.Now()

	_, err := client.Region(context.Background())
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	elapsed := time.Since(start)
	if elapsed >= backoff {
		t.Fatalf("Region() elapsed %v, want less than backoff %v", elapsed, backoff)
	}

	requireEqual(t, "attempts", attempts.Load(), int32(1))
}

func TestHTTPClientContextCanceledDuringBackoff(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	attemptCh := make(chan struct{}, 1)

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		if attempts.Add(1) > 1 {
			t.Fatalf("unexpected retry attempt: %d", attempts.Load())
		}

		select {
		case attemptCh <- struct{}{}:
		default:
		}

		return newHTTPResponse(
			http.StatusServiceUnavailable,
			io.NopCloser(strings.NewReader("retryable")),
			req,
		), nil
	}))

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL("http://metadata.local/opc/v2"),
		imds.WithMaxAttempts(2),
		imds.WithBackoff(500*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	errCh := startRegionRequest(ctx, t, client)

	<-attemptCh
	time.AfterFunc(10*time.Millisecond, cancel)

	err := waitForRegionError(t, errCh, "Region() during backoff")
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Region() error = %v, want wrapped context cancellation", err)
	}

	if !strings.Contains(err.Error(), "retry wait for region") {
		t.Fatalf("Region() error = %v, want retry wait cancellation", err)
	}

	requireEqual(t, "attempts", attempts.Load(), int32(1))
}

func TestHTTPClientRetryBudgetExhaustedWrapsLastError(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		attempts.Add(1)

		return nil, errDialFailure
	}))

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL("http://metadata.local/opc/v2"),
		imds.WithMaxAttempts(2),
		imds.WithBackoff(10*time.Millisecond),
	)

	_, err := client.Region(context.Background())
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	if !strings.Contains(err.Error(), "exhausted retry budget") {
		t.Fatalf("Region() error = %v, want exhausted retry budget", err)
	}

	if !errors.Is(err, errDialFailure) {
		t.Fatalf("Region() error = %v, want wrapped dial failure", err)
	}

	requireEqual(t, "attempts", attempts.Load(), int32(2))
}

func TestHTTPClientReadFailureIncludesCloseError(t *testing.T) {
	t.Parallel()

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		return newHTTPResponse(
			http.StatusOK,
			&faultyReadCloser{
				readErr:  io.ErrUnexpectedEOF,
				closeErr: errCloseBoom,
			},
			req,
		), nil
	}))

	client := imds.NewClient(httpClient, imds.WithBaseURL("http://metadata.local/opc/v2"))

	_, err := client.Region(context.Background())
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	if !strings.Contains(err.Error(), "read region response") {
		t.Fatalf("Region() error = %v, want read error", err)
	}

	if !strings.Contains(err.Error(), "close response body") {
		t.Fatalf("Region() error = %v, want close error joined", err)
	}

	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Region() error = %v, want read error propagated", err)
	}

	if !errors.Is(err, errCloseBoom) {
		t.Fatalf("Region() error = %v, want close error propagated", err)
	}
}

func TestHTTPClientCloseFailure(t *testing.T) {
	t.Parallel()

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		return newHTTPResponse(
			http.StatusOK,
			&staticBody{
				data:     []byte("us-london-1"),
				once:     sync.Once{},
				closeErr: errCloseFailed,
			},
			req,
		), nil
	}))

	client := imds.NewClient(httpClient, imds.WithBaseURL("http://metadata.local/opc/v2"))

	_, err := client.Region(context.Background())
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	if !strings.Contains(err.Error(), "close region response body") {
		t.Fatalf("Region() error = %v, want close failure", err)
	}

	if !errors.Is(err, errCloseFailed) {
		t.Fatalf("Region() error = %v, want close failure propagated", err)
	}
}

func TestHTTPClientCloseFailureWrapsCloseError(t *testing.T) {
	t.Parallel()

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		return newHTTPResponse(
			http.StatusOK,
			&faultyReadCloser{
				readErr:  io.EOF,
				closeErr: errCloseBoom,
			},
			req,
		), nil
	}))

	client := imds.NewClient(httpClient, imds.WithBaseURL("http://metadata.local/opc/v2"))

	_, err := client.Region(context.Background())
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	if !errors.Is(err, errCloseBoom) {
		t.Fatalf("Region() error = %v, want close error", err)
	}

	if !strings.Contains(err.Error(), "close region response body") {
		t.Fatalf("Region() error = %v, want close failure", err)
	}
}

func TestHTTPClientNonRetryableStatusIncludesBody(t *testing.T) {
	t.Parallel()

	server := newIPv4TestServer(
		t,
		http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
			requireIMDSAuthHeader(t, req)

			writer.WriteHeader(http.StatusBadRequest)
			_, _ = writer.Write([]byte(" not found \n"))
		}),
	)
	t.Cleanup(server.Close)

	httpClient := server.Client()
	httpClient.Timeout = time.Second

	client := imds.NewClient(httpClient, imds.WithBaseURL(server.URL+"/opc/v2"))

	_, err := client.Region(context.Background())
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	if !strings.Contains(err.Error(), "body not found") {
		t.Fatalf("Region() error = %v, want trimmed body", err)
	}
}

func TestHTTPClientNonRetryableStatusStopsRetryAndIncludesBody(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		attempts.Add(1)

		return newHTTPResponse(
			http.StatusBadRequest,
			io.NopCloser(strings.NewReader("  bad request \n")),
			req,
		), nil
	}))

	client := imds.NewClient(httpClient, imds.WithBaseURL("http://metadata.local/opc/v2"))

	_, err := client.Region(context.Background())
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	if strings.Contains(err.Error(), "exhausted retry budget") {
		t.Fatalf("Region() error = %v, did not expect retry budget exhaustion", err)
	}

	if !strings.Contains(err.Error(), "body bad request") {
		t.Fatalf("Region() error = %v, want trimmed body", err)
	}

	requireEqual(t, "attempts", attempts.Load(), int32(1))
}

func TestHTTPClientRetryBudgetExhaustedIncludesLastError(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	server := newIPv4TestServer(
		t,
		http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
			requireIMDSAuthHeader(t, req)

			attempts.Add(1)
			writer.WriteHeader(http.StatusTooManyRequests)
		}),
	)
	t.Cleanup(server.Close)

	httpClient := server.Client()
	httpClient.Timeout = time.Second

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL(server.URL+"/opc/v2"),
		imds.WithMaxAttempts(2),
		imds.WithBackoff(10*time.Millisecond),
	)

	_, err := client.Region(context.Background())
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	if !strings.Contains(err.Error(), "exhausted retry budget") {
		t.Fatalf("Region() error = %v, want exhausted retry budget", err)
	}

	if !strings.Contains(err.Error(), "retryable status code") {
		t.Fatalf("Region() error = %v, want last retryable status code", err)
	}

	requireEqual(t, "attempts", attempts.Load(), int32(2))
}

func TestHTTPClient429StatusTriggersRetry(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		switch attempts.Add(1) {
		case 1:
			return newHTTPResponse(
				http.StatusTooManyRequests,
				io.NopCloser(strings.NewReader("retry")),
				req,
			), nil
		default:
			return newHTTPResponse(
				http.StatusOK,
				io.NopCloser(strings.NewReader("us-phoenix-1")),
				req,
			), nil
		}
	}))

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL("http://metadata.local/opc/v2"),
		imds.WithMaxAttempts(2),
		imds.WithBackoff(5*time.Millisecond),
	)

	gotRegion, err := client.Region(context.Background())
	requireNoError(t, err, "Region()")

	requireEqual(t, "Region()", gotRegion, "us-phoenix-1")
	requireEqual(t, "attempts", attempts.Load(), int32(2))
}

func TestHTTPClientWaitHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	attemptCh := make(chan struct{})
	doneCh := make(chan struct{})

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		select {
		case attemptCh <- struct{}{}:
		default:
		}

		return newHTTPResponse(
			http.StatusServiceUnavailable,
			io.NopCloser(strings.NewReader("retry later")),
			req,
		), nil
	}))

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL("http://metadata.local/opc/v2"),
		imds.WithMaxAttempts(2),
		imds.WithBackoff(250*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer close(doneCh)

		_, _ = client.Region(ctx)
	}()

	<-attemptCh
	cancel()

	select {
	case <-doneCh:
	case <-time.After(time.Second):
		t.Fatal("Region() did not return after context cancellation")
	}
}

func TestHTTPClientFetchCanceledDuringRetryWait(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	attemptCh := make(chan struct{}, 1)

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		if attempts.Add(1) > 1 {
			t.Fatalf("unexpected retry attempt: %d", attempts.Load())
		}

		select {
		case attemptCh <- struct{}{}:
		default:
		}

		return newHTTPResponse(
			http.StatusServiceUnavailable,
			io.NopCloser(strings.NewReader("retryable")),
			req,
		), nil
	}))

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL("http://metadata.local/opc/v2"),
		imds.WithMaxAttempts(3),
		imds.WithBackoff(25*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := startRegionRequest(ctx, t, client)

	<-attemptCh

	cancelTimer := time.AfterFunc(5*time.Millisecond, cancel)
	defer cancelTimer.Stop()
	defer cancel()

	err := waitForRegionError(t, errCh, "Region() after canceling retry wait")
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Region() error = %v, want wrapped context cancellation", err)
	}

	if !strings.Contains(err.Error(), "retry wait for region") {
		t.Fatalf("Region() error = %v, want retry wait cancellation", err)
	}

	requireEqual(t, "attempts", attempts.Load(), int32(1))
}

func TestHTTPClientCancelDuringBackoffWithRetryableStatus(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	firstAttempt := make(chan struct{}, 1)

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		attempts.Add(1)

		select {
		case firstAttempt <- struct{}{}:
		default:
		}

		return newHTTPResponse(
			http.StatusServiceUnavailable,
			io.NopCloser(strings.NewReader("retryable")),
			req,
		), nil
	}))

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL("http://metadata.local/opc/v2"),
		imds.WithMaxAttempts(3),
		imds.WithBackoff(200*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	errCh := startRegionRequest(ctx, t, client)

	select {
	case <-firstAttempt:
	case <-time.After(time.Second):
		t.Fatal("Region() did not make the first attempt")
	}

	time.Sleep(25 * time.Millisecond)
	cancel()

	err := waitForRegionError(t, errCh, "Region() during backoff")
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Region() error = %v, want context cancellation", err)
	}

	if !strings.Contains(err.Error(), "retry wait for region") {
		t.Fatalf("Region() error = %v, want retry wait cancellation", err)
	}

	requireEqual(t, "attempts", attempts.Load(), int32(1))
}

func TestHTTPClientFetchContextCanceledDuringBackoff(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	attemptCh := make(chan struct{}, 1)

	httpClient := newHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireIMDSAuthHeader(t, req)

		attempts.Add(1)

		select {
		case attemptCh <- struct{}{}:
		default:
		}

		return nil, errDialFailure
	}))

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL("http://metadata.local/opc/v2"),
		imds.WithMaxAttempts(3),
		imds.WithBackoff(200*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() {
		<-attemptCh
		time.Sleep(25 * time.Millisecond)
		cancel()
	}()

	_, err := client.Region(ctx)
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Region() error = %v, want wrapped context cancellation", err)
	}

	if strings.Contains(err.Error(), "exhausted retry budget") {
		t.Fatalf("Region() error = %v, want context cancellation before exhausting retries", err)
	}

	requireEqual(t, "attempts", attempts.Load(), int32(1))
}

func TestHTTPClientCancelWhileWaitingToRetry(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	attemptCh := make(chan struct{}, 1)

	server := newRetryableStatusServer(t, &attempts, attemptCh)

	httpClient := server.Client()
	httpClient.Timeout = time.Second

	client := imds.NewClient(
		httpClient,
		imds.WithBaseURL(server.URL+"/opc/v2"),
		imds.WithMaxAttempts(2),
		imds.WithBackoff(250*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	errCh := startRegionRequest(ctx, t, client)

	select {
	case <-attemptCh:
	case <-time.After(time.Second):
		t.Fatal("Region() did not issue initial request")
	}

	time.Sleep(75 * time.Millisecond)
	cancel()

	err := waitForRegionError(t, errCh, "Region() while waiting to retry")
	if err == nil {
		t.Fatal("Region() expected error, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Region() error = %v, want wrapped context cancellation", err)
	}

	if !strings.Contains(err.Error(), "context done while waiting to retry") {
		t.Fatalf("Region() error = %v, want wait cancellation", err)
	}

	requireEqual(t, "attempts", attempts.Load(), int32(1))
}

func newRetryableStatusServer(
	t *testing.T,
	attempts *atomic.Int32,
	attemptCh chan struct{},
) *httptest.Server {
	t.Helper()

	server := newIPv4TestServer(
		t,
		http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
			requireIMDSAuthHeader(t, req)

			if attempts.Add(1) > 1 {
				t.Fatalf("unexpected retry attempt: %d", attempts.Load())
			}

			select {
			case attemptCh <- struct{}{}:
			default:
			}

			writer.WriteHeader(http.StatusServiceUnavailable)
		}),
	)
	t.Cleanup(server.Close)

	return server
}

func waitForRegionError(t *testing.T, errCh <-chan error, context string) error {
	t.Helper()

	select {
	case err := <-errCh:
		return err
	case <-time.After(time.Second):
		t.Fatalf("%s did not return", context)

		return nil
	}
}

func startRegionRequest(ctx context.Context, t *testing.T, client *imds.HTTPClient) chan error {
	t.Helper()

	errCh := make(chan error, 1)

	go func() {
		_, err := client.Region(ctx)
		errCh <- err
	}()

	return errCh
}
