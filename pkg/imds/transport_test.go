package imds_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/imds"
)

var (
	errDialFailure = errors.New("dial failure")
	errCloseBoom   = errors.New("close boom")
	errCloseFailed = errors.New("close failure")
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

	errCh := make(chan error, 1)

	go func() {
		_, err := client.Region(ctx)
		errCh <- err
	}()

	<-attemptCh
	cancel()

	err := <-errCh
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
