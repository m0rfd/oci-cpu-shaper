//nolint:testpackage // white-box tests exercise internal seams for coverage.
package e2eclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/oci"
)

func TestNewMonitoringClientValidatesEndpoint(t *testing.T) {
	t.Parallel()

	for name, endpoint := range map[string]string{
		"empty string":    "",
		"whitespace only": "  \n\t ",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := NewMonitoringClient(endpoint)
			if !errors.Is(err, errMonitoringEndpointRequired) {
				t.Fatalf("expected errMonitoringEndpointRequired, got %v", err)
			}
		})
	}
}

func TestNewMonitoringClientTrimsEndpointAndSetsTimeout(t *testing.T) {
	t.Parallel()

	input := "  http://example.com/metrics  "

	client, err := NewMonitoringClient(input)
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}

	if client.endpoint != strings.TrimSpace(input) {
		t.Fatalf("endpoint not trimmed: got %q", client.endpoint)
	}

	if client.http == nil {
		t.Fatalf("http client not initialised")
	}

	if client.http.Timeout != defaultHTTPTimeout {
		t.Fatalf("unexpected timeout: got %s want %s", client.http.Timeout, defaultHTTPTimeout)
	}
}

func TestMonitoringClientRejectsNilHTTPClient(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatalf("nil http client should prevent requests")
	}))
	t.Cleanup(server.Close)

	client := &MonitoringClient{endpoint: server.URL, http: nil}

	_, _, err := client.QueryP95CPU(context.Background(), "resource")
	if !errors.Is(err, errMonitoringHTTPNotInitialised) {
		t.Fatalf("expected errMonitoringHTTPNotInitialised, got %v", err)
	}
}

func TestMonitoringClientQueryP95CPURequestBuildFailure(t *testing.T) {
	t.Parallel()

	// Malformed endpoint forces http.NewRequestWithContext to fail during request construction.
	client := &MonitoringClient{endpoint: "http://[::1]:namedport", http: http.DefaultClient}

	_, _, err := client.QueryP95CPU(context.Background(), "resource")
	if err == nil || !strings.Contains(err.Error(), "build request") {
		t.Fatalf("expected wrapped build request error, got %v", err)
	}
}

func TestMonitoringClientQueryP95CPUHandlesNoContent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusNoContent)
		}),
	)
	t.Cleanup(server.Close)

	client, err := NewMonitoringClient(server.URL)
	if err != nil {
		t.Fatalf("unexpected client error: %v", err)
	}

	value, fetchedAt, err := client.QueryP95CPU(context.Background(), "resource")
	if !errors.Is(err, oci.ErrNoMetricsData) {
		t.Fatalf("expected ErrNoMetricsData, got %v", err)
	}

	if value != 0 {
		t.Fatalf("expected zero value on no content, got %.2f", value)
	}

	if !fetchedAt.IsZero() {
		t.Fatalf("expected zero timestamp on no content, got %v", fetchedAt)
	}
}

func TestMonitoringClientQueryP95CPUUnexpectedStatusEmptyBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusBadGateway)
		}),
	)
	t.Cleanup(server.Close)

	client, err := NewMonitoringClient(server.URL)
	if err != nil {
		t.Fatalf("unexpected client error: %v", err)
	}

	_, _, err = client.QueryP95CPU(context.Background(), "resource")
	if !errors.Is(err, errMonitoringUnexpectedStatus) {
		t.Fatalf("expected errMonitoringUnexpectedStatus, got %v", err)
	}

	if strings.Contains(err.Error(), "response body") {
		t.Fatalf("unexpected response body hint for empty response: %v", err)
	}
}

func TestMonitoringClientQueryP95CPUUnexpectedStatusTruncatesResponseBody(t *testing.T) {
	t.Parallel()

	oversizedBody := strings.Repeat("a", responseBodyLimit+64)

	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write([]byte(oversizedBody))
		}),
	)
	t.Cleanup(server.Close)

	client, err := NewMonitoringClient(server.URL)
	if err != nil {
		t.Fatalf("unexpected client error: %v", err)
	}

	_, _, err = client.QueryP95CPU(context.Background(), "resource")
	if !errors.Is(err, errMonitoringResponseBody) {
		t.Fatalf("expected errMonitoringResponseBody, got %v", err)
	}

	prefix := errMonitoringResponseBody.Error() + ": "

	trimmedBody := strings.TrimPrefix(err.Error(), prefix)
	if len(trimmedBody) != responseBodyLimit {
		t.Fatalf(
			"expected response body limited to %d bytes, got %d",
			responseBodyLimit,
			len(trimmedBody),
		)
	}

	if strings.Contains(trimmedBody, strings.Repeat("a", responseBodyLimit+1)) {
		t.Fatalf("response body should be truncated, got %q", trimmedBody)
	}
}

func TestMonitoringClientQueryP95CPUSuccessfulDecodeReturnsCurrentTimestamp(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.RawQuery != "resource=ok" {
				t.Fatalf("unexpected query: %s", request.URL.RawQuery)
			}

			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`{"value":2.5}`))
		}),
	)
	t.Cleanup(server.Close)

	client, err := NewMonitoringClient(server.URL)
	if err != nil {
		t.Fatalf("unexpected client error: %v", err)
	}

	before := time.Now().UTC()

	value, fetchedAt, err := client.QueryP95CPU(context.Background(), "ok")
	if err != nil {
		t.Fatalf("unexpected success error: %v", err)
	}

	if value != 2.5 {
		t.Fatalf("unexpected value: got %.2f want 2.5", value)
	}

	after := time.Now().UTC()
	if fetchedAt.Before(before) || fetchedAt.After(after.Add(50*time.Millisecond)) {
		t.Fatalf(
			"expected timestamp between %v and %v, got %v",
			before,
			after.Add(50*time.Millisecond),
			fetchedAt,
		)
	}
}
