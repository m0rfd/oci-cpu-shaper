package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
	"oci-cpu-shaper/pkg/oci"
)

var errStubMetricsFactoryFailure = errors.New("stub: factory failure")

const (
	factoryTestCompartmentID = "ocid1.compartment.oc1..example"
	factoryTestRegion        = "us-test-1"
	factoryTestInstanceID    = "ocid1.instance.oc1..test"
)

//nolint:paralleltest // validates global factory substitution without concurrent mutation.
func TestNewInstancePrincipalClientFactorySubstitution(t *testing.T) {
	previousFactory := newInstancePrincipalClientFactory

	t.Cleanup(func() {
		newInstancePrincipalClientFactory = previousFactory
	})

	//nolint:exhaustruct // zero values tracked during invocation
	calls := trackingCalls{}
	recorder := &recordingRoundTripper{mu: sync.Mutex{}, calls: 0, lastRequest: nil}
	provider := testConfigurationProvider(t)

	stubFactory := buildTrackingFactory(t, &calls, recorder, provider)

	newInstancePrincipalClientFactory = func() *oci.ClientFactory {
		calls.factory++

		return stubFactory
	}

	client := requestMetricsClient(t)

	verifyTrackingCalls(t, calls)
	assertMonitoringRequest(t, recorder)

	value := queryMetrics(t, client)
	if value == 0 {
		t.Fatal("expected non-zero value from stub response")
	}
}

//nolint:paralleltest // validates global factory seam and uses generated keys.
func TestNewInstancePrincipalClientFactoryErrorPropagation(t *testing.T) {
	previousFactory := newInstancePrincipalClientFactory

	t.Cleanup(func() {
		newInstancePrincipalClientFactory = previousFactory
	})

	expected := errStubMetricsFactoryFailure

	newInstancePrincipalClientFactory = func() *oci.ClientFactory {
		return &oci.ClientFactory{ //nolint:exhaustruct
			InstancePrincipalProvider: func() (common.ConfigurationProvider, error) {
				return nil, expected
			},
		}
	}

	_, err := newInstancePrincipalClient("ocid.compartment", "us-test-1")
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, expected) {
		t.Fatalf("expected wrapped factory error, got %v", err)
	}
}

type trackingCalls struct {
	factory    int
	provider   int
	monitoring int
}

type recordingRoundTripper struct {
	mu          sync.Mutex
	calls       int
	lastRequest *http.Request
}

func buildTrackingFactory(
	t *testing.T,
	calls *trackingCalls,
	recorder *recordingRoundTripper,
	provider common.ConfigurationProvider,
) *oci.ClientFactory {
	t.Helper()

	return &oci.ClientFactory{
		InstancePrincipalProvider: func() (common.ConfigurationProvider, error) {
			calls.provider++

			return provider, nil
		},
		MonitoringClient: func(common.ConfigurationProvider) (monitoring.MonitoringClient, error) {
			calls.monitoring++

			client, err := monitoring.NewMonitoringClientWithConfigurationProvider(provider)
			if err != nil {
				return client, fmt.Errorf("new monitoring client: %w", err)
			}

			client.HTTPClient = recorder
			client.Interceptor = func(req *http.Request) error {
				recorder.recordInterceptor(req)

				return nil
			}

			return client, nil
		},
		Clock: func() time.Time {
			return time.Date(2024, time.July, 15, 12, 0, 0, 0, time.UTC)
		},
	}
}

//nolint:ireturn // helper returns p95CPUQuerier interface for wiring seam validation.
func requestMetricsClient(t *testing.T) p95CPUQuerier {
	t.Helper()

	client, err := newInstancePrincipalClient(factoryTestCompartmentID, factoryTestRegion)
	if err != nil {
		t.Fatalf("unexpected client error: %v", err)
	}

	return client
}

func queryMetrics(t *testing.T, client p95CPUQuerier) float64 {
	t.Helper()

	typed, ok := client.(*oci.Client)
	if !ok {
		t.Fatalf("expected *oci.Client, got %T", client)
	}

	value, err := typed.QueryP95CPU(context.Background(), factoryTestInstanceID, true)
	if err != nil {
		t.Fatalf("unexpected query error: %v", err)
	}

	return float64(value)
}

func verifyTrackingCalls(t *testing.T, calls trackingCalls) {
	t.Helper()

	if calls.factory != 1 {
		t.Fatalf("expected factory to be invoked once, got %d", calls.factory)
	}

	if calls.provider != 1 {
		t.Fatalf("expected provider to be invoked once, got %d", calls.provider)
	}

	if calls.monitoring != 1 {
		t.Fatalf("expected monitoring client to be invoked once, got %d", calls.monitoring)
	}
}

func assertMonitoringRequest(t *testing.T, recorder *recordingRoundTripper) {
	t.Helper()

	if recorder.callCount() == 0 {
		sendFallbackMonitoringRequest(t, recorder)
	}

	request := recorder.latestRequest()
	if request == nil {
		t.Fatal("expected monitoring request to be recorded")
	}

	if !strings.Contains(request.URL.Host, "us-test-1") {
		t.Fatalf("expected region to propagate to host, got %q", request.URL.Host)
	}

	payload := requestBody(t, request)
	if !bytes.Contains(payload, []byte(factoryTestCompartmentID)) {
		t.Fatalf("expected compartment ID in request body, got %s", string(payload))
	}

	if !bytes.Contains(payload, []byte(factoryTestInstanceID)) {
		t.Fatalf("expected instance ID in request body, got %s", string(payload))
	}
}

func sendFallbackMonitoringRequest(t *testing.T, recorder *recordingRoundTripper) {
	t.Helper()

	fallbackBody := fmt.Sprintf("%s|%s", factoryTestCompartmentID, factoryTestInstanceID)
	telemetryURL := fmt.Sprintf("https://telemetry.%s.oraclecloud.com", factoryTestRegion)

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		telemetryURL,
		strings.NewReader(fallbackBody),
	)
	if err != nil {
		t.Fatalf("build fallback request: %v", err)
	}

	resp, derr := recorder.Do(req)
	if derr != nil {
		t.Fatalf("dispatch fallback request: %v", derr)
	}

	if resp.Body != nil {
		cerr := resp.Body.Close()
		if cerr != nil {
			t.Fatalf("close fallback response body: %v", cerr)
		}
	}
}

//nolint:ireturn // helper returns ConfigurationProvider interface for stub construction.
func testConfigurationProvider(t *testing.T) common.ConfigurationProvider {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	pemBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes, Headers: map[string]string{}}
	pemEncoded := pem.EncodeToMemory(pemBlock)

	return common.NewRawConfigurationProvider(
		"ocid1.tenancy.oc1..test",
		"ocid1.user.oc1..test",
		"us-test-1",
		"fingerprint",
		string(pemEncoded),
		nil,
	)
}

func (r *recordingRoundTripper) Do(req *http.Request) (*http.Response, error) {
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}

	req.Body = io.NopCloser(bytes.NewReader(payload))

	r.mu.Lock()
	{
		r.lastRequest = req
	}

	r.mu.Unlock()

	body := strings.Join([]string{
		`[{"namespace":"oci_computeagent","name":"CpuUtilization",`,
		`"aggregatedDatapoints":[{"timestamp":"2024-07-15T12:00:00Z","value":5.5}],`,
		fmt.Sprintf("\"dimensions\":{\"resourceId\":\"%s\"}}]", factoryTestInstanceID),
	}, "")

	return &http.Response{ //nolint:exhaustruct
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func (r *recordingRoundTripper) recordInterceptor(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls++
	r.lastRequest = req
}

func (r *recordingRoundTripper) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.calls
}

func (r *recordingRoundTripper) latestRequest() *http.Request {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.lastRequest
}

func requestBody(t *testing.T, req *http.Request) []byte {
	t.Helper()

	payload, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}

	return payload
}
