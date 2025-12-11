//nolint:testpackage // white-box tests exercise internal seams for coverage.
package e2eclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

//nolint:cyclop,funlen // multiple request/response paths validated in one scenario.
func TestMonitoringClientQueryP95CPUScenarios(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Query().Get("resource") {
			case "empty":
				writer.WriteHeader(http.StatusNoContent)
			case "status-only":
				writer.WriteHeader(http.StatusServiceUnavailable)
			case "error":
				writer.WriteHeader(http.StatusServiceUnavailable)
				_, _ = writer.Write([]byte("backend unavailable"))
			case "invalid":
				writer.WriteHeader(http.StatusOK)
				_, _ = writer.Write([]byte("not-json"))
			default:
				writer.WriteHeader(http.StatusOK)
				_, _ = writer.Write([]byte(`{"value":0.42}`))
			}
		}),
	)
	t.Cleanup(server.Close)

	client, err := NewMonitoringClient(server.URL)
	if err != nil {
		t.Fatalf("unexpected client error: %v", err)
	}

	_, fetchedAt, err := client.QueryP95CPU(context.Background(), "empty")
	if !errors.Is(err, oci.ErrNoMetricsData) {
		t.Fatalf("expected ErrNoMetricsData, got %v", err)
	}

	if !fetchedAt.IsZero() {
		t.Fatalf("expected zero timestamp on empty response, got %v", fetchedAt)
	}

	_, _, err = client.QueryP95CPU(context.Background(), "status-only")
	if !errors.Is(err, errMonitoringUnexpectedStatus) {
		t.Fatalf("expected errMonitoringUnexpectedStatus, got %v", err)
	}

	_, _, err = client.QueryP95CPU(context.Background(), "error")
	if !errors.Is(err, errMonitoringResponseBody) ||
		!strings.Contains(err.Error(), "backend unavailable") {
		t.Fatalf("expected backend error, got %v", err)
	}

	_, _, err = client.QueryP95CPU(context.Background(), "invalid")
	if err == nil || !strings.Contains(err.Error(), "decode payload") {
		t.Fatalf("expected decode error, got %v", err)
	}

	value, fetchedAt, err := client.QueryP95CPU(context.Background(), "ok")
	if err != nil {
		t.Fatalf("unexpected success error: %v", err)
	}

	if fetchedAt.IsZero() {
		t.Fatalf("expected non-zero timestamp on success")
	}

	if value != 0.42 {
		t.Fatalf("unexpected value: got %.2f want 0.42", value)
	}
}

func TestMonitoringClientQueryP95CPUNilReceiver(t *testing.T) {
	t.Parallel()

	var client *MonitoringClient

	_, _, err := client.QueryP95CPU(context.Background(), "resource")
	if !errors.Is(err, errMonitoringHTTPNotInitialised) {
		t.Fatalf("expected errMonitoringHTTPNotInitialised, got %v", err)
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

func TestMonitoringClientQueryP95CPUWrapsUnexpectedStatus(t *testing.T) {
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

	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected status code in error, got %v", err)
	}
}

func TestMonitoringClientQueryP95CPUWrapsResponseBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write([]byte("backend unavailable\n"))
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

	if !strings.Contains(err.Error(), "backend unavailable") {
		t.Fatalf("expected response body to propagate, got %v", err)
	}
}

func TestMonitoringClientRejectsUninitialisedHTTPClient(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatalf("nil client should not issue requests")
	}))
	t.Cleanup(server.Close)

	client := &MonitoringClient{endpoint: server.URL, http: nil}

	_, _, err := client.QueryP95CPU(context.Background(), "resource")
	if !errors.Is(err, errMonitoringHTTPNotInitialised) {
		t.Fatalf("expected errMonitoringHTTPNotInitialised, got %v", err)
	}
}
