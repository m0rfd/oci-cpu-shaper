package imds //nolint:testpackage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

var errTemporaryDialFailure = errors.New("temporary dial failure")

func TestHTTPClientTryFetchRetryableStatus(t *testing.T) {
	t.Parallel()

	var closed bool

	//nolint:bodyclose // tryFetch closes stubbed responses.
	httpClient := newStubHTTPClient(
		[]*http.Response{
			newStubResponse(http.StatusBadGateway, &trackingReadCloser{
				Reader: strings.NewReader("retry"),
				closed: &closed,
			}), //nolint:bodyclose
		},
		[]error{nil},
		nil,
	)

	client := &HTTPClient{
		http:       httpClient,
		baseURL:    "http://metadata.local/opc/v2",
		maxAttempt: 1,
		backoff:    0,
	}

	_, retry, err := client.tryFetch(context.Background(), "region")
	if err == nil {
		t.Fatal("tryFetch expected error, got nil")
	}

	if !errors.Is(err, errRetryableStatus) {
		t.Fatalf("tryFetch error = %v, want errRetryableStatus", err)
	}

	if !retry {
		t.Fatalf("tryFetch retry = %v, want true", retry)
	}

	if !closed {
		t.Fatalf("tryFetch did not close response body")
	}
}

func TestHTTPClientTryFetchUnexpectedStatusTrimsBody(t *testing.T) {
	t.Parallel()

	var closed bool

	//nolint:bodyclose // tryFetch closes stubbed responses.
	httpClient := newStubHTTPClient(
		[]*http.Response{
			newStubResponse(
				http.StatusBadRequest,
				&trackingReadCloser{
					Reader: strings.NewReader("  bad request \n"),
					closed: &closed,
				},
			), //nolint:bodyclose
		},
		[]error{nil},
		nil,
	)

	client := &HTTPClient{
		http:       httpClient,
		baseURL:    "http://metadata.local/opc/v2",
		maxAttempt: 1,
		backoff:    0,
	}

	_, retry, err := client.tryFetch(context.Background(), "id")
	if err == nil {
		t.Fatal("tryFetch expected error, got nil")
	}

	if retry {
		t.Fatalf("tryFetch retry = %v, want false", retry)
	}

	if !errors.Is(err, errUnexpectedStatus) {
		t.Fatalf("tryFetch error = %v, want errUnexpectedStatus", err)
	}

	if !strings.Contains(err.Error(), "body bad request") {
		t.Fatalf("tryFetch error = %v, want trimmed body", err)
	}

	if !closed {
		t.Fatalf("tryFetch did not close response body")
	}
}

func TestHTTPClientTryFetchDoErrorAfterContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	httpClient := newStubHTTPClient(nil, []error{context.Canceled}, func(*http.Request) {
		cancel()
	})

	client := &HTTPClient{
		http:       httpClient,
		baseURL:    "http://metadata.local/opc/v2",
		maxAttempt: 1,
		backoff:    0,
	}

	_, retry, err := client.tryFetch(ctx, "compartmentId")
	if err == nil {
		t.Fatal("tryFetch expected error, got nil")
	}

	if retry {
		t.Fatalf("tryFetch retry = %v, want false", retry)
	}

	if !errors.Is(err, errRequestFailed) {
		t.Fatalf("tryFetch error = %v, want errRequestFailed", err)
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("tryFetch error = %v, want wrapped context cancellation", err)
	}
}

func TestHTTPClientFetchRetryableDoErrorExhaustsBudget(t *testing.T) {
	t.Parallel()

	httpClient := newStubHTTPClient(
		nil,
		[]error{errTemporaryDialFailure, errTemporaryDialFailure, errTemporaryDialFailure},
		nil,
	)

	client := &HTTPClient{
		http:       httpClient,
		baseURL:    "http://metadata.local/opc/v2",
		maxAttempt: 3,
		backoff:    time.Millisecond,
	}

	_, err := client.fetch(context.Background(), "region")
	if err == nil {
		t.Fatal("fetch expected error, got nil")
	}

	if !errors.Is(err, errExhaustedRetries) {
		t.Fatalf("fetch error = %v, want errExhaustedRetries", err)
	}

	if !errors.Is(err, errRequestFailed) {
		t.Fatalf("fetch error = %v, want errRequestFailed", err)
	}

	if !errors.Is(err, errTemporaryDialFailure) {
		t.Fatalf("fetch error = %v, want wrapped retry error", err)
	}

	roundTripper, ok := httpClient.Transport.(*stubRoundTripper)
	if !ok {
		t.Fatalf("unexpected transport type: %T", httpClient.Transport)
	}

	if roundTripper.calls != 3 {
		t.Fatalf("fetch attempts = %d, want 3", roundTripper.calls)
	}
}

func TestHTTPClientFetchCanceledWhileWaitingToRetry(t *testing.T) {
	t.Parallel()

	attemptCh := make(chan struct{}, 1)
	httpClient := newRetryableHTTPClient(t, attemptCh)

	client := &HTTPClient{
		http:       httpClient,
		baseURL:    "http://metadata.local/opc/v2",
		maxAttempt: 2,
		backoff:    100 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	errCh := make(chan error, 1)

	go func() {
		_, err := client.fetch(ctx, "region")
		errCh <- err
	}()

	<-attemptCh

	time.Sleep(10 * time.Millisecond)
	cancel()

	err := awaitFetchError(t, errCh, "fetch after cancelation")
	if err == nil {
		t.Fatal("fetch expected error, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("fetch error = %v, want wrapped context cancellation", err)
	}

	if !strings.Contains(err.Error(), "retry wait for region") {
		t.Fatalf("fetch error = %v, want retry wait context", err)
	}

	roundTripper, ok := httpClient.Transport.(*stubRoundTripper)
	if !ok {
		t.Fatalf("unexpected transport type: %T", httpClient.Transport)
	}

	if roundTripper.calls != 1 {
		t.Fatalf("fetch attempts = %d, want 1", roundTripper.calls)
	}
}

func newRetryableHTTPClient(t *testing.T, attemptCh chan<- struct{}) *http.Client {
	t.Helper()

	responses := []*http.Response{
		newStubResponse( //nolint:bodyclose // fetch closes stub responses.
			http.StatusServiceUnavailable,
			io.NopCloser(strings.NewReader("retry")),
		),
		newStubResponse( //nolint:bodyclose // fetch closes stub responses.
			http.StatusServiceUnavailable,
			io.NopCloser(strings.NewReader("retry")),
		),
	}

	t.Cleanup(func() {
		for _, response := range responses {
			_ = response.Body.Close()
		}
	})

	return newStubHTTPClient(
		responses,
		nil,
		func(*http.Request) {
			select {
			case attemptCh <- struct{}{}:
			default:
			}
		},
	)
}

func awaitFetchError(t *testing.T, errCh <-chan error, context string) error {
	t.Helper()

	select {
	case err := <-errCh:
		return err
	case <-time.After(time.Second):
		t.Fatalf("%s did not return", context)

		return nil
	}
}

func newStubHTTPClient(
	responses []*http.Response,
	errs []error,
	onCall func(*http.Request),
) *http.Client {
	return &http.Client{ //nolint:exhaustruct
		Transport: &stubRoundTripper{
			responses: responses,
			errs:      errs,
			onCall:    onCall,
			calls:     0,
		},
	}
}

type stubRoundTripper struct {
	responses []*http.Response
	errs      []error
	onCall    func(*http.Request)
	calls     int
}

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if s.onCall != nil {
		s.onCall(req)
	}

	idx := s.calls
	s.calls++

	var response *http.Response
	if idx < len(s.responses) {
		response = s.responses[idx]
	}

	var err error
	if idx < len(s.errs) {
		err = s.errs[idx]
	}

	return response, err
}

func newStubResponse(statusCode int, body io.ReadCloser) *http.Response {
	req, _ := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"http://metadata.local/opc/v2/instance",
		nil,
	)

	return &http.Response{ //nolint:exhaustruct
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       body,
		Request:    req,
	}
}

type trackingReadCloser struct {
	io.Reader

	closed *bool
}

func (t *trackingReadCloser) Close() error {
	if t.closed != nil {
		*t.closed = true
	}

	return nil
}
