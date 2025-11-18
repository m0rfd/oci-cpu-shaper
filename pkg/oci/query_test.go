package oci //nolint:testpackage

import (
	"context"
	"encoding/json"
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

func TestQueryP95CPUFetchesLatestDatapoint(t *testing.T) {
	t.Parallel()

	instanceID := "ocid1.instance.oc1.phx.exampleuniqueID"
	compartmentID := "ocid1.compartment.oc1..exampleuniqueID"
	now := time.Date(2025, time.January, 2, 15, 4, 5, 0, time.UTC)

	expectedQuery := "CpuUtilization[1m]{resourceId = \"" + instanceID + "\"}.percentile(0.95)"

	server := newIPv4TestServer(
		t,
		http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
			t.Helper()

			defer func() {
				_ = req.Body.Close()
			}()

			var payload map[string]string

			err := json.NewDecoder(req.Body).Decode(&payload)
			requireNoError(t, err, "decode payload")

			requireEqual(t, payload["query"], expectedQuery, "unexpected query")

			if payload["startTime"] == "" || payload["endTime"] == "" {
				t.Fatalf("expected start and end time in payload: %#v", payload)
			}

			writer.WriteHeader(http.StatusOK)
		}),
	)
	t.Cleanup(server.Close)

	responses := []monitoring.SummarizeMetricsDataResponse{
		metricResponse(metricData(instanceID, compartmentID, now.Add(-10*time.Minute), 12.5)),
		metricResponse(metricData(instanceID, compartmentID, now.Add(-5*time.Minute), 18.75)),
	}

	verifying := newHTTPVerifyingClient(t, server, responses, []string{"next"})

	client, err := newTestClient(verifying, compartmentID, func() time.Time { return now })
	requireNoError(t, err, "create client")

	value, err := client.QueryP95CPU(context.Background(), instanceID, true)
	requireNoError(t, err, "QueryP95CPU")

	requireEqual(t, value, float32(18.75), "unexpected value")

	verifying.mu.Lock()
	defer verifying.mu.Unlock()

	requireEqual(t, len(verifying.requests), 2, "request count")
	assertRequestWindow(t, verifying.requests[0], now.Add(-7*24*time.Hour), now)

	requireEqual(t, len(verifying.pages), 2, "page count")
	requireEqual(t, verifying.pages[0], "", "first page token")
	requireEqual(t, verifying.pages[1], "next", "second page token")
}

func TestQueryP95CPUHandlesMissingData(t *testing.T) {
	t.Parallel()

	server := newIPv4TestServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(server.Close)

	verifying := newHTTPVerifyingClient(
		t,
		server,
		[]monitoring.SummarizeMetricsDataResponse{metricResponse()},
		nil,
	)

	client, err := newTestClient(
		verifying,
		"ocid1.compartment.oc1..exampleuniqueID",
		func() time.Time {
			return time.Now().UTC()
		},
	)
	requireNoError(t, err, "create client")

	_, err = client.QueryP95CPU(context.Background(), "ocid1.instance.oc1.phx.empty", false)
	if !errors.Is(err, ErrNoMetricsData) {
		t.Fatalf("expected ErrNoMetricsData, got %v", err)
	}
}

func TestQueryP95CPUPropagatesErrors(t *testing.T) {
	t.Parallel()

	server := newIPv4TestServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(server.Close)

	verifying := newHTTPVerifyingClient(t, server, nil, nil)
	verifying.err = errForcedFailure

	client, err := newTestClient(verifying, "ocid1.compartment.oc1..exampleuniqueID", time.Now)
	requireNoError(t, err, "create client")

	_, err = client.QueryP95CPU(context.Background(), "ocid1.instance.oc1.phx.failure", false)
	if err == nil || !strings.Contains(err.Error(), "summarize metrics") {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

func TestComputeWindowRespectsLookbackLimits(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, time.July, 1, 15, 30, 45, 123456789, time.UTC)

	t.Run("24h-window", func(t *testing.T) {
		t.Parallel()

		t.Helper()

		start, end := computeWindow(now, false)

		requireEqual(t, end, now.Truncate(time.Second), "end timestamp truncated")
		requireEqual(t, start, end.Add(-24*time.Hour), "24h lookback")
	})

	t.Run("7d-window-truncated", func(t *testing.T) {
		t.Parallel()

		t.Helper()

		start, end := computeWindow(now, true)

		expectedWindow := time.Duration(maxOneMinuteWindowHours) * time.Hour

		requireEqual(t, end, now.Truncate(time.Second), "end timestamp truncated")
		requireEqual(t, start, end.Add(-expectedWindow), "seven day lookback")
	})
}

func TestBuildSummarizeRequestEscapesInstanceOCID(t *testing.T) {
	t.Parallel()

	start := time.Date(2024, time.June, 30, 14, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)

	compartmentID := "ocid1.compartment.oc1..exampleuniqueID"
	instanceID := "ocid1.instance.oc1..example\"uniqueID"

	request := buildSummarizeRequest(compartmentID, instanceID, start, end)

	if request.CompartmentId == nil {
		t.Fatalf("request missing compartment ID: %#v", request)
	}

	requireEqual(t, *request.CompartmentId, compartmentID, "compartment ID")

	details := request.SummarizeMetricsDataDetails

	if details.Query == nil {
		t.Fatalf("request missing query: %#v", details)
	}

	expectedQuery := fmt.Sprintf(metricQueryTemplate, escapeDimensionValue(instanceID))
	requireEqual(t, *details.Query, expectedQuery, "escaped query")

	if details.StartTime == nil || details.EndTime == nil {
		t.Fatalf("request missing timestamps: %#v", details)
	}

	requireEqual(t, details.StartTime.Time, start, "start time")
	requireEqual(t, details.EndTime.Time, end, "end time")
}

func TestCollectLatestDatapointAggregatesAcrossPages(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, time.June, 30, 16, 0, 0, 0, time.UTC)

	responses := []monitoring.SummarizeMetricsDataResponse{
		metricResponse(
			metricData("ocid.instance", "ocid.compartment", now.Add(-90*time.Minute), 10.0),
			metricData("ocid.instance", "ocid.compartment", now.Add(-45*time.Minute), 12.5),
			metricDataWithNilFields(),
		),
		metricResponse(
			metricData("ocid.instance", "ocid.compartment", now.Add(-15*time.Minute), 18.75),
		),
	}

	tokens := []*string{
		stringPointer(" next-page "),
		stringPointer("   "),
	}

	stub := newStubMetricsClient(responses, tokens, nil)

	client, err := newTestClient(stub, "ocid.compartment", func() time.Time { return now })
	requireNoError(t, err, "create client")

	request := buildSummarizeRequest(
		"ocid.compartment",
		"ocid.instance",
		now.Add(-2*time.Hour),
		now,
	)

	value, found, err := client.collectLatestDatapoint(context.Background(), request)
	requireNoError(t, err, "collect datapoint")

	if !found {
		t.Fatalf("expected to find datapoint")
	}

	requireEqual(t, value, float32(18.75), "latest datapoint")

	if stub.calls != 2 {
		t.Fatalf("expected 2 API calls, got %d", stub.calls)
	}
}

func TestCollectLatestDatapointHandlesEmptyResponses(t *testing.T) {
	t.Parallel()

	stub := newStubMetricsClient(
		[]monitoring.SummarizeMetricsDataResponse{metricResponse()},
		nil,
		nil,
	)

	client, err := newTestClient(stub, "ocid.compartment", time.Now)
	requireNoError(t, err, "create client")

	request := buildSummarizeRequest(
		"ocid.compartment",
		"ocid.instance",
		time.Now().Add(-time.Hour),
		time.Now(),
	)

	_, found, err := client.collectLatestDatapoint(context.Background(), request)
	requireNoError(t, err, "collect datapoint")

	if found {
		t.Fatalf("expected no datapoint to be found")
	}
}

func TestCollectLatestDatapointPropagatesErrors(t *testing.T) {
	t.Parallel()

	stub := newStubMetricsClient(nil, nil, errForcedFailure)

	client, err := newTestClient(stub, "ocid.compartment", time.Now)
	requireNoError(t, err, "create client")

	request := buildSummarizeRequest(
		"ocid.compartment",
		"ocid.instance",
		time.Now().Add(-time.Hour),
		time.Now(),
	)

	_, _, err = client.collectLatestDatapoint(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "summarize metrics") {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

func TestNormalizePageToken(t *testing.T) {
	t.Parallel()

	if token := normalizePageToken(nil); token != nil {
		t.Fatalf("expected nil for nil token, got %#v", token)
	}

	whitespace := "  \t  "
	if token := normalizePageToken(&whitespace); token != nil {
		t.Fatalf("expected nil for whitespace token, got %#v", token)
	}

	raw := " next "

	token := normalizePageToken(&raw)
	if token == nil || *token != "next" {
		t.Fatalf("expected trimmed token 'next', got %#v", token)
	}
}

func TestEscapeDimensionValue(t *testing.T) {
	t.Parallel()

	input := `ocid1.instance.oc1..example"uniqueID`
	expected := `ocid1.instance.oc1..example\"uniqueID`

	requireEqual(t, escapeDimensionValue(input), expected, "escaped value")
}

func TestNewClientValidatesParameters(t *testing.T) {
	t.Parallel()

	_, err := newClient(nil, "ocid.compartment", time.Now)
	if !errors.Is(err, errMissingMetricsClient) {
		t.Fatalf("expected errMissingMetricsClient, got %v", err)
	}

	_, err = newClient(newStubMetricsClient(nil, nil, nil), "", time.Now)
	if !errors.Is(err, errMissingCompartmentID) {
		t.Fatalf("expected errMissingCompartmentID, got %v", err)
	}

	client, err := newClient(newStubMetricsClient(nil, nil, nil), "ocid.compartment", nil)
	requireNoError(t, err, "create client with default clock")

	if client == nil || client.now == nil {
		t.Fatalf("expected client with default clock, got %#v", client)
	}
}

func TestNewInstancePrincipalClientPropagatesProviderError(t *testing.T) {
	t.Parallel()

	overrideInstancePrincipalProvider(t, func() (common.ConfigurationProvider, error) {
		return nil, errForcedFailure
	})

	_, err := NewInstancePrincipalClient("ocid1.compartment.oc1..exampleuniqueID", "us-ashburn-1")
	if err == nil || !strings.Contains(err.Error(), "build instance principal provider") {
		t.Fatalf("expected wrapped provider error, got %v", err)
	}
}

func TestNewInstancePrincipalClientPropagatesClientError(t *testing.T) {
	t.Parallel()

	provider := stubConfigurationProvider(t)

	overrideInstancePrincipalProvider(t, func() (common.ConfigurationProvider, error) {
		return provider, nil
	})

	overrideNewMonitoringClient(
		t,
		func(common.ConfigurationProvider) (monitoring.MonitoringClient, error) {
			var client monitoring.MonitoringClient

			return client, errForcedFailure
		},
	)

	_, err := NewInstancePrincipalClient("ocid1.compartment.oc1..exampleuniqueID", "us-ashburn-1")
	if err == nil || !strings.Contains(err.Error(), "create monitoring client") {
		t.Fatalf("expected monitoring client error, got %v", err)
	}
}

func TestNewInstancePrincipalClientSuccess(t *testing.T) {
	t.Parallel()

	provider := stubConfigurationProvider(t)

	overrideInstancePrincipalProvider(t, func() (common.ConfigurationProvider, error) {
		return provider, nil
	})

	overrideNewMonitoringClient(
		t,
		func(common.ConfigurationProvider) (monitoring.MonitoringClient, error) {
			var client monitoring.MonitoringClient

			return client, nil
		},
	)

	client, err := NewInstancePrincipalClient(
		"ocid1.compartment.oc1..exampleuniqueID",
		"us-ashburn-1",
	)
	requireNoError(t, err, "create instance principal client")

	if client == nil {
		t.Fatalf("expected client instance")
	}
}

func TestSDKMonitoringClientSummarizeMetricsDataDecodesResponse(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, time.June, 30, 18, 45, 0, 0, time.UTC)

	body, err := json.Marshal([]map[string]any{
		{
			"namespace":     monitoringNamespace,
			"resourceGroup": nil,
			"name":          metricName,
			"dimensions": map[string]string{
				"resourceId": "ocid.instance",
			},
			"aggregatedDatapoints": []map[string]any{
				{
					"timestamp": now.Format(time.RFC3339),
					"value":     42.5,
				},
			},
		},
	})
	requireNoError(t, err, "marshal summary response")

	response := newJSONResponse(
		string(body),
		http.Header{
			"Content-Type":  []string{"application/json"},
			"Opc-Next-Page": []string{" token "},
		},
	)

	t.Cleanup(func() {
		_ = response.Body.Close()
	})

	caller := newStubAPICaller(response, nil)

	client := &sdkMonitoringClient{client: caller}

	request := buildSummarizeRequest(
		"ocid.compartment",
		"ocid.instance",
		now.Add(-time.Minute),
		now,
	)

	summaryResponse, next, err := client.SummarizeMetricsData(
		context.Background(),
		request,
		stringPointer(" token "),
	)
	requireNoError(t, err, "summarize metrics")

	assertSummaryDatapoint(t, summaryResponse, now, 42.5)
	assertNextPageToken(t, next, "token")
	assertSummarizeRequest(t, caller.lastRequest, "token")
}

func TestSDKMonitoringClientSummarizeMetricsDataWrapsCallErrors(t *testing.T) {
	t.Parallel()

	caller := newStubAPICaller(nil, errForcedFailure)

	client := &sdkMonitoringClient{client: caller}

	request := buildSummarizeRequest(
		"ocid.compartment",
		"ocid.instance",
		time.Now().Add(-time.Minute),
		time.Now(),
	)

	_, _, err := client.SummarizeMetricsData(context.Background(), request, nil)
	if err == nil || !strings.Contains(err.Error(), "execute summarize metrics request") {
		t.Fatalf("expected wrapped call error, got %v", err)
	}
}

func TestSDKMonitoringClientSummarizeMetricsDataHandlesDecodeErrors(t *testing.T) {
	t.Parallel()

	response := new(http.Response)
	response.StatusCode = http.StatusOK
	response.Header = http.Header{"Content-Type": []string{"application/json"}}
	response.Body = io.NopCloser(strings.NewReader("not-json"))

	caller := newStubAPICaller(response, nil)

	client := &sdkMonitoringClient{client: caller}

	request := buildSummarizeRequest(
		"ocid.compartment",
		"ocid.instance",
		time.Now().Add(-time.Minute),
		time.Now(),
	)

	_, _, err := client.SummarizeMetricsData(context.Background(), request, nil)
	if err == nil || !strings.Contains(err.Error(), "decode summarize metrics response") {
		t.Fatalf("expected decode error, got %v", err)
	}
}
