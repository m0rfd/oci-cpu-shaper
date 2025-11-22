package oci //nolint:testpackage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

var (
	errNoMockResponse = errors.New("http mock: no response configured")
	errForcedFailure  = errors.New("http mock: forced failure")
)

type httpVerifyingClient struct {
	t          *testing.T
	endpoint   string
	httpClient *http.Client

	mu         sync.Mutex
	requests   []monitoring.SummarizeMetricsDataRequest
	responses  []monitoring.SummarizeMetricsDataResponse
	pages      []string
	nextTokens []string
	err        error
}

func newHTTPVerifyingClient(
	t *testing.T,
	server *httptest.Server,
	responses []monitoring.SummarizeMetricsDataResponse,
	nextTokens []string,
) *httpVerifyingClient {
	t.Helper()

	return &httpVerifyingClient{
		t:          t,
		endpoint:   server.URL,
		httpClient: server.Client(),
		mu:         sync.Mutex{},
		requests:   nil,
		responses:  append([]monitoring.SummarizeMetricsDataResponse{}, responses...),
		pages:      nil,
		nextTokens: append([]string(nil), nextTokens...),
		err:        nil,
	}
}

func (c *httpVerifyingClient) SummarizeMetricsData(
	ctx context.Context,
	request monitoring.SummarizeMetricsDataRequest,
	page *string,
) (monitoring.SummarizeMetricsDataResponse, *string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.err != nil {
		return monitoring.SummarizeMetricsDataResponse{}, nil, c.err
	}

	payload, token := buildRequestPayload(request, page)

	err := c.postPayload(ctx, payload)
	if err != nil {
		return monitoring.SummarizeMetricsDataResponse{}, nil, err
	}

	c.requests = append(c.requests, request)
	c.pages = append(c.pages, token)

	if len(c.responses) == 0 {
		return monitoring.SummarizeMetricsDataResponse{}, nil, errNoMockResponse
	}

	response := c.responses[0]
	c.responses = c.responses[1:]

	if len(c.nextTokens) == 0 {
		return response, nil, nil
	}

	next := strings.TrimSpace(c.nextTokens[0])
	c.nextTokens = c.nextTokens[1:]

	if next == "" {
		return response, nil, nil
	}

	return response, &next, nil
}

func buildRequestPayload(
	request monitoring.SummarizeMetricsDataRequest,
	page *string,
) (map[string]any, string) {
	payload := map[string]any{}
	details := request.SummarizeMetricsDataDetails

	if details.Query != nil {
		payload["query"] = *details.Query
	}

	if details.StartTime != nil {
		payload["startTime"] = details.StartTime.Format(time.RFC3339)
	}

	if details.EndTime != nil {
		payload["endTime"] = details.EndTime.Format(time.RFC3339)
	}

	trimmed := ""
	if page != nil {
		trimmed = strings.TrimSpace(*page)
		if trimmed != "" {
			payload["page"] = trimmed
		}
	}

	return payload, trimmed
}

func (c *httpVerifyingClient) postPayload(ctx context.Context, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	httpRequest.Header.Set("Content-Type", "application/json")

	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("issue mock request: %w", err)
	}

	defer func() {
		_ = httpResponse.Body.Close()
	}()

	return nil
}

func metricData(
	instanceID, compartmentID string,
	timestamp time.Time,
	value float64,
) monitoring.MetricData {
	var datapoint monitoring.AggregatedDatapoint

	datapoint.Timestamp = &common.SDKTime{Time: timestamp}
	datapoint.Value = common.Float64(value)

	var data monitoring.MetricData

	data.Namespace = common.String(monitoringNamespace)
	data.CompartmentId = common.String(compartmentID)
	data.Name = common.String(metricName)
	data.Dimensions = map[string]string{"resourceId": instanceID}
	data.AggregatedDatapoints = []monitoring.AggregatedDatapoint{datapoint}
	data.ResourceGroup = nil
	data.Metadata = nil
	data.Resolution = nil

	return data
}

func metricResponse(items ...monitoring.MetricData) monitoring.SummarizeMetricsDataResponse {
	var response monitoring.SummarizeMetricsDataResponse

	response.RawResponse = nil
	response.Items = append(response.Items, items...)
	response.OpcRequestId = nil

	return response
}

func metricDataWithNilFields() monitoring.MetricData {
	var datapoint monitoring.AggregatedDatapoint

	datapoint.Timestamp = nil
	datapoint.Value = nil

	var data monitoring.MetricData

	data.Namespace = common.String(monitoringNamespace)
	data.AggregatedDatapoints = []monitoring.AggregatedDatapoint{datapoint}

	return data
}

func TestCollectLatestDatapointSkipsWhitespaceNextPageHeaders(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, time.January, 15, 12, 0, 0, 0, time.UTC)
	compartmentID := testCompartmentID
	instanceID := "ocid1.instance.oc1..exampleuniqueID"

	server, requestCount := newWhitespacePagingServer(t, instanceID, compartmentID, now)
	t.Cleanup(server.Close)

	client, err := newTestClient(
		&sdkMonitoringClient{client: newServerCaller(t, server)},
		compartmentID,
		func() time.Time { return now },
	)
	requireNoError(t, err, "create client")

	request := buildSummarizeRequest(compartmentID, instanceID, now.Add(-time.Hour), now)

	value, found, err := client.collectLatestDatapoint(context.Background(), request)
	requireNoError(t, err, "collect datapoint")

	if !found {
		t.Fatalf("expected datapoint to be found")
	}

	requireEqual(t, value, float32(20.0), "latest datapoint")

	if *requestCount != 1 {
		t.Fatalf("expected one page to be fetched, got %d", *requestCount)
	}
}

func newWhitespacePagingServer(
	t *testing.T,
	instanceID, compartmentID string,
	now time.Time,
) (*httptest.Server, *int) {
	t.Helper()

	firstPagePayload := marshalMetricsPayload(
		t,
		metricData(instanceID, compartmentID, now.Add(-15*time.Minute), 12.5),
		metricData(instanceID, compartmentID, now.Add(-5*time.Minute), 20.0),
	)

	secondPagePayload := marshalMetricsPayload(
		t,
		metricData(instanceID, compartmentID, now, 99.0),
	)

	requestCount := 0

	server := newIPv4TestServer(
		t,
		http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
			t.Helper()

			requestCount++

			assertSummarizeRequest(t, req, "")
			assertQueryPayload(t, req, compartmentID, instanceID)

			writer.Header().Set("Content-Type", "application/json")

			switch requestCount {
			case 1:
				writer.Header().Set("Opc-Next-Page", "  \t ")

				_, err := writer.Write(firstPagePayload)
				requireNoError(t, err, "write first page")
			case 2:
				_, err := writer.Write(secondPagePayload)
				requireNoError(t, err, "write second page")
			default:
				t.Fatalf("unexpected request %d with query %q", requestCount, req.URL.RawQuery)
			}
		}),
	)

	return server, &requestCount
}

func marshalMetricsPayload(t *testing.T, metrics ...monitoring.MetricData) []byte {
	t.Helper()

	payload, err := json.Marshal(metrics)
	requireNoError(t, err, "marshal metrics payload")

	return payload
}

func assertQueryPayload(
	t *testing.T,
	req *http.Request,
	compartmentID, instanceID string,
) {
	t.Helper()

	query := req.URL.Query()

	requireEqual(t, query.Get("compartmentId"), compartmentID, "compartment query")
	requireEqual(t, query.Get("page"), "", "page query")

	var payload map[string]any

	err := json.NewDecoder(req.Body).Decode(&payload)
	requireNoError(t, err, "decode request payload")

	queryExpression, ok := payload["query"].(string)
	if !ok || !strings.Contains(queryExpression, instanceID) {
		t.Fatalf(
			"expected query payload to reference instance %q, got %#v",
			instanceID,
			payload["query"],
		)
	}
}

func requireNoError(t *testing.T, err error, message string) {
	t.Helper()

	if err != nil {
		t.Fatalf("%s: %v", message, err)
	}
}

func requireEqual[T comparable](t *testing.T, got, want T, message string) {
	t.Helper()

	if got != want {
		t.Fatalf("%s: got %v want %v", message, got, want)
	}
}

func assertRequestWindow(
	t *testing.T,
	request monitoring.SummarizeMetricsDataRequest,
	start, end time.Time,
) {
	t.Helper()

	details := request.SummarizeMetricsDataDetails
	if details.StartTime == nil || details.EndTime == nil {
		t.Fatalf("request missing timestamps: %#v", details)
	}

	requireEqual(t, details.StartTime.Time, start, "unexpected start time")
	requireEqual(t, details.EndTime.Time, end, "unexpected end time")
}

func newStubMetricsClient(
	responses []monitoring.SummarizeMetricsDataResponse,
	tokens []*string,
	err error,
) *stubMetricsClient {
	copiedResponses := append([]monitoring.SummarizeMetricsDataResponse(nil), responses...)
	copiedTokens := append([]*string(nil), tokens...)

	return &stubMetricsClient{
		responses: copiedResponses,
		tokens:    copiedTokens,
		err:       err,
		calls:     0,
	}
}

type stubMetricsClient struct {
	responses []monitoring.SummarizeMetricsDataResponse
	tokens    []*string
	err       error

	calls int
}

func (s *stubMetricsClient) SummarizeMetricsData(
	_ context.Context,
	_ monitoring.SummarizeMetricsDataRequest,
	_ *string,
) (monitoring.SummarizeMetricsDataResponse, *string, error) {
	s.calls++

	if s.err != nil {
		return monitoring.SummarizeMetricsDataResponse{}, nil, s.err
	}

	if len(s.responses) == 0 {
		return monitoring.SummarizeMetricsDataResponse{}, nil, errNoMockResponse
	}

	response := s.responses[0]
	s.responses = s.responses[1:]

	var next *string
	if len(s.tokens) > 0 {
		next = s.tokens[0]
		s.tokens = s.tokens[1:]
	}

	return response, next, nil
}

func stringPointer(value string) *string {
	return &value
}

func newJSONResponse(body string, headers http.Header) *http.Response {
	response := new(http.Response)
	response.StatusCode = http.StatusOK
	response.Header = headers.Clone()
	response.Body = io.NopCloser(strings.NewReader(body))
	response.ContentLength = int64(len(body))

	return response
}

func assertNextPageToken(t *testing.T, token *string, expected string) {
	t.Helper()

	if token == nil || *token != expected {
		t.Fatalf("expected next page token %q, got %#v", expected, token)
	}
}

func assertSummarizeRequest(t *testing.T, request *http.Request, expectedPage string) {
	t.Helper()

	if request == nil {
		t.Fatalf("expected request to be recorded")
	}

	requireEqual(t, request.URL.Path, "/metrics/actions/summarizeMetricsData", "request path")
	requireEqual(t, request.URL.Query().Get("page"), expectedPage, "page query")
}

func assertSummaryDatapoint(
	t *testing.T,
	summary monitoring.SummarizeMetricsDataResponse,
	expectedTimestamp time.Time,
	expectedValue float64,
) {
	t.Helper()

	if len(summary.Items) != 1 {
		t.Fatalf("expected one metric item, got %d", len(summary.Items))
	}

	datapoints := summary.Items[0].AggregatedDatapoints
	if len(datapoints) != 1 {
		t.Fatalf("expected one datapoint, got %d", len(datapoints))
	}

	requireEqual(t, datapoints[0].Timestamp.Time, expectedTimestamp, "datapoint timestamp")
	requireEqual(t, float32(*datapoints[0].Value), float32(expectedValue), "datapoint value")
}

// newIPv4TestServer binds to the IPv4 loopback explicitly so tests still work when
// the sandbox forbids listening on IPv6.
func newIPv4TestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	server := httptest.NewUnstartedServer(handler)

	var lc net.ListenConfig

	listener, err := lc.Listen(context.Background(), "tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp4: %v", err)
	}

	server.Listener = listener
	server.Start()

	return server
}

type serverCaller struct {
	serverURL *url.URL
	client    *http.Client
}

func (s *serverCaller) Call(ctx context.Context, req *http.Request) (*http.Response, error) {
	cloned := req.Clone(ctx)

	cloned.URL.Scheme = s.serverURL.Scheme
	cloned.URL.Host = s.serverURL.Host
	cloned.Host = s.serverURL.Host

	response, err := s.client.Do(cloned)
	if err != nil {
		return nil, fmt.Errorf("issue mock request: %w", err)
	}

	return response, nil
}

func newServerCaller(t *testing.T, server *httptest.Server) *serverCaller {
	t.Helper()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	return &serverCaller{
		serverURL: serverURL,
		client:    server.Client(),
	}
}
