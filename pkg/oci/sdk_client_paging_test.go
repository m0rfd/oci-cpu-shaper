package oci //nolint:testpackage

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

type sequenceAPICaller struct {
	responses []*http.Response
	closes    []*bool
	requests  []*http.Request

	index int
}

//nolint:revive // context argument retained for interface parity
func (s *sequenceAPICaller) Call(
	ctx context.Context,
	req *http.Request,
) (*http.Response, error) {
	s.requests = append(s.requests, req)

	if s.index >= len(s.responses) {
		return nil, errNoMockResponse
	}

	response := s.responses[s.index]
	s.index++

	return response, nil
}

type trackingBody struct {
	*strings.Reader

	closed *bool
}

func (b *trackingBody) Close() error {
	*b.closed = true

	return nil
}

//nolint:bodyclose // responses are closed by the method under test
func buildTrackingResponses(t *testing.T, pages []mockPage) ([]*http.Response, []*bool) {
	t.Helper()

	responses := make([]*http.Response, 0, len(pages))
	closes := make([]*bool, 0, len(pages))

	for _, page := range pages {
		body, err := json.Marshal([]monitoring.MetricData{metricData(
			"ocid.instance", "ocid.compartment", page.timestamp, page.value,
		)})
		requireNoError(t, err, "marshal page body")

		headers := http.Header{"Content-Type": []string{"application/json"}}
		if page.nextHeader != "" {
			headers.Set("Opc-Next-Page", page.nextHeader)
		}

		response := newJSONResponse(string(body), headers)

		closed := false
		response.Body = &trackingBody{Reader: strings.NewReader(string(body)), closed: &closed}
		response.ContentLength = int64(len(body))

		responses = append(responses, response)
		closes = append(closes, &closed)
	}

	return responses, closes
}

type mockPage struct {
	timestamp  time.Time
	value      float64
	nextHeader string
}

type pagingTestCase struct {
	name          string
	initialPage   *string
	expectedPages []string
	pages         []mockPage
	expectedFinal *string
}

func TestSDKMonitoringClientSummarizeMetricsDataPaging(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, time.November, 5, 12, 0, 0, 0, time.UTC)

	testCases := []pagingTestCase{
		{
			name:          "no next page",
			initialPage:   nil,
			expectedPages: []string{""},
			pages: []mockPage{{
				timestamp:  now,
				value:      1,
				nextHeader: "",
			}},
			expectedFinal: nil,
		},
		{
			name:          "whitespace tokens normalized",
			initialPage:   stringPointer("   "),
			expectedPages: []string{""},
			pages: []mockPage{{
				timestamp:  now.Add(time.Minute),
				value:      2,
				nextHeader: "   ",
			}},
			expectedFinal: nil,
		},
		{
			name:          "multiple pages",
			initialPage:   nil,
			expectedPages: []string{"", "token-1"},
			pages: []mockPage{
				{
					timestamp:  now.Add(2 * time.Minute),
					value:      3,
					nextHeader: " token-1 ",
				},
				{
					timestamp:  now.Add(3 * time.Minute),
					value:      4,
					nextHeader: " final-token ",
				},
			},
			expectedFinal: stringPointer("final-token"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runSummarizePagingCase(t, now, testCase)
		})
	}
}

func runSummarizePagingCase(t *testing.T, now time.Time, testCase pagingTestCase) {
	t.Helper()

	responses, closes := buildTrackingResponses(t, testCase.pages) //nolint:bodyclose

	caller := &sequenceAPICaller{
		responses: responses,
		closes:    closes,
		requests:  nil,
		index:     0,
	}
	client := &sdkMonitoringClient{client: caller}

	request := buildSummarizeRequest(
		"ocid.compartment",
		"ocid.instance",
		now, now.Add(time.Minute),
	)

	pageToken := testCase.initialPage

	var lastToken *string

	for callIndex, expectedPage := range testCase.expectedPages {
		summary, next, err := client.SummarizeMetricsData(
			context.Background(), request, pageToken,
		)
		requireNoError(t, err, "summarize metrics")

		assertSummaryDatapoint(
			t,
			summary,
			testCase.pages[callIndex].timestamp,
			testCase.pages[callIndex].value,
		)
		assertSummarizeRequest(t, caller.requests[callIndex], expectedPage)

		pageToken = next
		lastToken = next
	}

	assertNormalizedToken(t, lastToken, testCase.expectedFinal)

	for i, closed := range closes {
		if closed == nil || !*closed {
			t.Fatalf("response %d body was not closed", i)
		}
	}
}

func assertNormalizedToken(t *testing.T, token *string, expected *string) {
	t.Helper()

	switch {
	case expected == nil && token == nil:
		return
	case expected == nil && token != nil:
		t.Fatalf("expected nil token, got %q", *token)
	case expected != nil && token == nil:
		t.Fatalf("expected token %q, got nil", *expected)
	case *expected != *token:
		t.Fatalf("expected token %q, got %q", *expected, *token)
	}
}
