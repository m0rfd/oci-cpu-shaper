package imds

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestMetadataRequestSetsAuthorizationHeader(t *testing.T) {
	t.Parallel()

	req, err := metadataRequest(context.Background(), http.MethodGet, "http://example.com")
	if err != nil {
		t.Fatalf("metadataRequest returned error: %v", err)
	}

	if got := req.Header.Get("Authorization"); got != metadataAuthorization {
		t.Fatalf("expected authorization header %q, got %q", metadataAuthorization, got)
	}
}

func TestMetadataRequestPropagatesBuilderError(t *testing.T) {
	t.Parallel()

	_, err := metadataRequest(context.Background(), "with spaces", "http://example.com")
	if err == nil {
		t.Fatal("expected metadataRequest to fail for invalid method")
	}
}

func TestNewClientAppliesOptions(t *testing.T) {
	t.Parallel()

	httpClient := newTestHTTPClient()

	client := NewClient(
		httpClient,
		WithBaseURL("  http://example.com/opc/v2/  "),
		WithMaxAttempts(7),
		WithBackoff(123*time.Millisecond),
	)

	httpMetadataClient, ok := client.(*HTTPClient)
	if !ok {
		t.Fatalf("expected *HTTPClient, got %T", client)
	}

	if httpMetadataClient.http != httpClient {
		t.Fatal("expected provided http.Client to be used")
	}

	expectedBase := "http://example.com/opc/v2"
	if httpMetadataClient.baseURL != expectedBase {
		t.Fatalf("expected baseURL %q, got %q", expectedBase, httpMetadataClient.baseURL)
	}

	if httpMetadataClient.maxAttempt != 7 {
		t.Fatalf("expected maxAttempt 7, got %d", httpMetadataClient.maxAttempt)
	}

	if httpMetadataClient.backoff != 123*time.Millisecond {
		t.Fatalf("expected backoff 123ms, got %s", httpMetadataClient.backoff)
	}
}

func TestNewClientUsesDefaultsForNilInputs(t *testing.T) {
	t.Parallel()

	client := NewClient(nil, nil)

	httpMetadataClient, ok := client.(*HTTPClient)
	if !ok {
		t.Fatalf("expected *HTTPClient, got %T", client)
	}

	if httpMetadataClient.baseURL != DefaultEndpoint {
		t.Fatalf("expected default baseURL %q, got %q", DefaultEndpoint, httpMetadataClient.baseURL)
	}

	if httpMetadataClient.maxAttempt != defaultMaxAttempts {
		t.Fatalf(
			"expected default maxAttempt %d, got %d",
			defaultMaxAttempts,
			httpMetadataClient.maxAttempt,
		)
	}

	if httpMetadataClient.backoff != defaultBackoff {
		t.Fatalf("expected default backoff %s, got %s", defaultBackoff, httpMetadataClient.backoff)
	}

	if httpMetadataClient.http == nil {
		t.Fatal("expected default http.Client to be created")
	}

	if httpMetadataClient.http.Timeout != defaultHTTPClientTimeout {
		t.Fatalf(
			"expected default timeout %s, got %s",
			defaultHTTPClientTimeout,
			httpMetadataClient.http.Timeout,
		)
	}
}

func TestNewClientIgnoresInvalidOptionValues(t *testing.T) {
	t.Parallel()

	httpClient := newTestHTTPClient()

	client := NewClient(
		httpClient,
		WithBaseURL("   \t"),
		WithMaxAttempts(0),
		WithBackoff(0),
	)

	httpMetadataClient, ok := client.(*HTTPClient)
	if !ok {
		t.Fatalf("expected *HTTPClient, got %T", client)
	}

	if httpMetadataClient.baseURL != DefaultEndpoint {
		t.Fatalf(
			"expected baseURL to remain %q, got %q",
			DefaultEndpoint,
			httpMetadataClient.baseURL,
		)
	}

	if httpMetadataClient.maxAttempt != defaultMaxAttempts {
		t.Fatalf(
			"expected maxAttempt to remain %d, got %d",
			defaultMaxAttempts,
			httpMetadataClient.maxAttempt,
		)
	}

	if httpMetadataClient.backoff != defaultBackoff {
		t.Fatalf(
			"expected backoff to remain %s, got %s",
			defaultBackoff,
			httpMetadataClient.backoff,
		)
	}
}

func newTestHTTPClient() *http.Client {
	return &http.Client{
		Transport:     http.DefaultTransport,
		CheckRedirect: http.DefaultClient.CheckRedirect,
		Jar:           http.DefaultClient.Jar,
		Timeout:       defaultHTTPClientTimeout,
	}
}
