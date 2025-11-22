package oci //nolint:testpackage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

var errCallFailed = errors.New("call failed")

type fakeAPICaller struct {
	response *http.Response
	err      error
	called   bool
}

func newTrackingResponse(statusCode int, body string, closed *bool) *http.Response {
	statusText := http.StatusText(statusCode)

	return &http.Response{
		Status:           fmt.Sprintf("%d %s", statusCode, statusText),
		StatusCode:       statusCode,
		Proto:            "HTTP/1.1",
		ProtoMajor:       1,
		ProtoMinor:       1,
		Header:           http.Header{"Content-Type": []string{"application/json"}},
		Body:             &trackingBody{Reader: strings.NewReader(body), closed: closed},
		ContentLength:    int64(len(body)),
		TransferEncoding: nil,
		Close:            false,
		Uncompressed:     false,
		Trailer:          nil,
		Request:          nil,
		TLS:              nil,
	}
}

//nolint:revive // context argument retained for interface parity
func (f *fakeAPICaller) Call(ctx context.Context, req *http.Request) (*http.Response, error) {
	f.called = true

	return f.response, f.err
}

func TestSDKMonitoringClientSummarizeMetricsDataBuildsRequest(t *testing.T) {
	t.Parallel()

	caller := &fakeAPICaller{response: nil, err: nil, called: false}
	client := &sdkMonitoringClient{client: caller}

	request := monitoring.SummarizeMetricsDataRequest{
		CompartmentId: nil,
		SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
			Namespace:     nil,
			Query:         nil,
			ResourceGroup: nil,
			StartTime:     nil,
			EndTime:       nil,
			Resolution:    nil,
		},
		OpcRequestId:           nil,
		CompartmentIdInSubtree: nil,
		RequestMetadata:        common.RequestMetadata{RetryPolicy: nil},
	}

	_, next, err := client.SummarizeMetricsData(context.Background(), request, nil)
	if err == nil || !strings.Contains(err.Error(), "build summarize request") {
		t.Fatalf("expected HTTPRequest error with context, got %v", err)
	}

	if next != nil {
		t.Fatalf("expected nil page token on request build failure, got %v", *next)
	}

	if caller.called {
		t.Fatalf("unexpected Call invocation when HTTPRequest failed")
	}
}

func TestSDKMonitoringClientSummarizeMetricsDataWrapsCallErrorsAndClosesBody(t *testing.T) {
	t.Parallel()

	closed := false
	response := newTrackingResponse(http.StatusInternalServerError, "{}", &closed)

	t.Cleanup(func() {
		if !closed {
			t.Fatalf("response body was not closed by SummarizeMetricsData")
		}

		_ = response.Body.Close()
	})

	caller := &fakeAPICaller{response: response, err: errCallFailed, called: false}
	client := &sdkMonitoringClient{client: caller}

	request := buildSummarizeRequest(
		"ocid.compartment",
		"ocid.instance",
		time.Now().Add(-time.Minute),
		time.Now(),
	)

	_, next, err := client.SummarizeMetricsData(context.Background(), request, nil)
	if err == nil || !strings.Contains(err.Error(), "execute summarize metrics request") {
		t.Fatalf("expected wrapped call error, got %v", err)
	}

	wrapped := common.PostProcessServiceError(
		errCallFailed,
		"Monitoring",
		"SummarizeMetricsData",
		"https://docs.oracle.com/iaas/api/#/en/monitoring/20180401/MetricData/SummarizeMetricsData",
	)

	if !errors.Is(err, wrapped) {
		t.Fatalf("expected error to wrap PostProcessServiceError result")
	}

	if !caller.called {
		t.Fatalf("expected Call to be invoked")
	}

	if !closed {
		t.Fatalf("expected response body to be closed when Call returns an error")
	}

	if next != nil {
		t.Fatalf("expected nil page token on call error, got %v", *next)
	}
}

func TestSDKMonitoringClientSummarizeMetricsDataHandlesDecodeErrors(t *testing.T) {
	t.Parallel()

	closed := false
	response := newTrackingResponse(http.StatusOK, "not-json", &closed)

	t.Cleanup(func() {
		if !closed {
			t.Fatalf("response body was not closed by SummarizeMetricsData")
		}

		_ = response.Body.Close()
	})

	caller := &fakeAPICaller{response: response, err: nil, called: false}
	client := &sdkMonitoringClient{client: caller}

	request := buildSummarizeRequest(
		"ocid.compartment",
		"ocid.instance",
		time.Now().Add(-time.Minute),
		time.Now(),
	)

	_, next, err := client.SummarizeMetricsData(context.Background(), request, nil)
	if err == nil || !strings.Contains(err.Error(), "decode summarize metrics response") {
		t.Fatalf("expected decode error, got %v", err)
	}

	if !caller.called {
		t.Fatalf("expected Call to be invoked")
	}

	if !closed {
		t.Fatalf("expected response body to be closed on decode error")
	}

	if next != nil {
		t.Fatalf("expected nil page token on decode error, got %v", *next)
	}
}
