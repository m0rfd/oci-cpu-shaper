package oci //nolint:testpackage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

const (
	testCompartmentID = "ocid1.compartment.oc1..exampleuniqueID"
)

// testTimeBuffer is the allowed clock skew or test execution delay.
// Chosen as a reasonable upper bound for drift to avoid flaky tests.
const testTimeBuffer = 2 * time.Second

func TestClientValidationErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var nilClient *Client

	_, err := nilClient.QueryP95CPU(ctx, "ocid.instance", false)
	if !errors.Is(err, errNilClient) {
		t.Fatalf("expected errNilClient, got %v", err)
	}

	_, err = newClient(nil, "ocid.compartment", time.Now)
	if !errors.Is(err, errMissingMetricsClient) {
		t.Fatalf("expected errMissingMetricsClient, got %v", err)
	}

	_, err = newClient(newStubMetricsClient(nil, nil, nil), "", time.Now)
	if !errors.Is(err, errMissingCompartmentID) {
		t.Fatalf("expected errMissingCompartmentID, got %v", err)
	}

	client, err := newTestClient(newStubMetricsClient(nil, nil, nil), "ocid.compartment", time.Now)
	requireNoError(t, err, "create client")

	_, err = client.QueryP95CPU(ctx, "", false)
	if !errors.Is(err, errMissingInstanceOCID) {
		t.Fatalf("expected errMissingInstanceOCID, got %v", err)
	}

	_, err = NewInstancePrincipalClient("", "us-phoenix-1")
	if !errors.Is(err, errMissingCompartmentID) {
		t.Fatalf("expected errMissingCompartmentID, got %v", err)
	}
}

func TestNewInstancePrincipalClientClockFallback(t *testing.T) {
	t.Parallel()

	before := time.Now()

	client, roundTripper := newRecordingInstancePrincipalClient(t, "us-phoenix-1")

	after := time.Now()

	now := client.now()
	if now.Before(before) || now.After(after.Add(testTimeBuffer)) {
		t.Fatalf("expected clock fallback near now, got %v", now)
	}

	value, err := client.QueryP95CPU(
		context.Background(),
		"ocid1.instance.oc1..exampleuniqueID",
		false,
	)
	requireNoError(t, err, "query P95 CPU")

	requireEqual(t, value, float32(37.5), "latest datapoint")

	if roundTripper.host == "" {
		t.Fatalf("expected request host to be recorded")
	}
}

func TestNewInstancePrincipalClientTrimsRegionOverride(t *testing.T) {
	t.Parallel()

	client, roundTripper := newRecordingInstancePrincipalClient(t, "  us-phoenix-1  ")

	_, err := client.QueryP95CPU(context.Background(), "ocid1.instance.oc1..exampleuniqueID", false)
	requireNoError(t, err, "query P95 CPU")

	if strings.Contains(roundTripper.host, " ") {
		t.Fatalf("expected trimmed region in host, got %q", roundTripper.host)
	}

	if !strings.Contains(roundTripper.host, "us-phoenix-1") {
		t.Fatalf("expected host to include trimmed region, got %q", roundTripper.host)
	}
}

func newRecordingInstancePrincipalClient(
	t *testing.T,
	region string,
) (*Client, *recordingRoundTripper) {
	t.Helper()

	provider := stubConfigurationProvider(t)

	roundTripper := &recordingRoundTripper{
		host:   "",
		status: http.StatusOK,
		body:   `[{"aggregatedDatapoints":[{"timestamp":"2024-07-01T00:00:00Z","value":37.5}]}]`,
	}

	factory := &ClientFactory{
		InstancePrincipalProvider: func() (common.ConfigurationProvider, error) {
			return provider, nil
		},
		MonitoringClient: func(p common.ConfigurationProvider) (monitoring.MonitoringClient, error) {
			client, err := monitoring.NewMonitoringClientWithConfigurationProvider(p)
			requireNoError(t, err, "build monitoring client")

			client.HTTPClient = &http.Client{
				Transport:     roundTripper,
				CheckRedirect: nil,
				Jar:           nil,
				Timeout:       0,
			}

			return client, nil
		},
		Clock: nil,
	}

	client, err := NewInstancePrincipalClient(testCompartmentID, region, WithFactory(factory))
	requireNoError(t, err, "create instance principal client")

	return client, roundTripper
}

type recordingRoundTripper struct {
	host   string
	status int
	body   string
}

func (r *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.host = req.URL.Host

	response := &http.Response{
		Status:           fmt.Sprintf("%d %s", r.status, http.StatusText(r.status)),
		StatusCode:       r.status,
		Proto:            "HTTP/1.1",
		ProtoMajor:       1,
		ProtoMinor:       1,
		Header:           http.Header{"Content-Type": []string{"application/json"}},
		Body:             io.NopCloser(strings.NewReader(r.body)),
		ContentLength:    int64(len(r.body)),
		TransferEncoding: nil,
		Close:            false,
		Uncompressed:     false,
		Trailer:          nil,
		Request:          req,
		TLS:              nil,
	}

	return response, nil
}
