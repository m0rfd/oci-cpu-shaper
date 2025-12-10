package imds //nolint:testpackage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

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
