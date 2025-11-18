package oci

import (
	"context"
	"fmt"
	"net/http"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

type ociAPICaller interface {
	Call(ctx context.Context, request *http.Request) (*http.Response, error)
}

type sdkMonitoringClient struct {
	client ociAPICaller
}

func (s *sdkMonitoringClient) SummarizeMetricsData(
	ctx context.Context,
	request monitoring.SummarizeMetricsDataRequest,
	page *string,
) (monitoring.SummarizeMetricsDataResponse, *string, error) {
	httpRequest, err := request.HTTPRequest(
		http.MethodPost,
		"/metrics/actions/summarizeMetricsData",
		nil,
		nil,
	)
	if err != nil {
		return monitoring.SummarizeMetricsDataResponse{}, nil, fmt.Errorf(
			"build summarize request: %w",
			err,
		)
	}

	if trimmed := normalizePageToken(page); trimmed != nil {
		query := httpRequest.URL.Query()
		query.Set("page", *trimmed)
		httpRequest.URL.RawQuery = query.Encode()
	}

	httpResponse, err := s.client.Call(ctx, &httpRequest)

	if httpResponse != nil {
		defer func() {
			common.CloseBodyIfValid(httpResponse)
		}()
	}

	var response monitoring.SummarizeMetricsDataResponse

	response.RawResponse = httpResponse

	if err != nil {
		apiReferenceLink := "https://docs.oracle.com/iaas/api/#/en/monitoring/20180401/MetricData/SummarizeMetricsData"
		wrapped := common.PostProcessServiceError(
			err,
			"Monitoring",
			"SummarizeMetricsData",
			apiReferenceLink,
		)

		return response, nil, fmt.Errorf("execute summarize metrics request: %w", wrapped)
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	if err != nil {
		return response, nil, fmt.Errorf("decode summarize metrics response: %w", err)
	}

	headerValue := httpResponse.Header.Get("Opc-Next-Page")

	return response, normalizePageToken(&headerValue), nil
}
