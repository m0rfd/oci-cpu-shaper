package oci

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

const (
	monitoringNamespace     = "oci_computeagent"
	metricQueryTemplate     = "CpuUtilization[1m]{resourceId = \"%s\"}.percentile(0.95)"
	metricName              = "CpuUtilization"
	maxOneMinuteWindowHours = 7 * 24
)

// QueryP95CPU returns the most recent P95 CpuUtilization datapoint for the supplied compute instance.
// The query spans the trailing seven days at one-minute resolution to match the reclaim horizon and
// the Monitoring API's resolution limit. ErrNoMetricsData is returned when the API yields no
// datapoints.
func (c *Client) QueryP95CPU(
	ctx context.Context,
	instanceOCID string,
) (float32, error) {
	if c == nil {
		return 0, errNilClient
	}

	if instanceOCID == "" {
		return 0, errMissingInstanceOCID
	}

	start, end := computeWindow(c.now().UTC())
	request := buildSummarizeRequest(c.compartmentID, instanceOCID, start, end)

	value, found, err := c.collectLatestDatapoint(ctx, request)
	if err != nil {
		return 0, err
	}

	if !found {
		return 0, ErrNoMetricsData
	}

	return value, nil
}

func computeWindow(now time.Time) (time.Time, time.Time) {
	end := now.Truncate(time.Second)

	maxWindow := time.Duration(maxOneMinuteWindowHours) * time.Hour
	start := end.Add(-maxWindow)

	return start, end
}

func buildSummarizeRequest(
	compartmentID, instanceOCID string,
	start, end time.Time,
) monitoring.SummarizeMetricsDataRequest {
	namespace := monitoringNamespace
	query := fmt.Sprintf(metricQueryTemplate, escapeDimensionValue(instanceOCID))
	startTime := common.SDKTime{Time: start}
	endTime := common.SDKTime{Time: end}

	var details monitoring.SummarizeMetricsDataDetails

	details.Namespace = &namespace
	details.Query = &query
	details.StartTime = &startTime
	details.EndTime = &endTime

	var request monitoring.SummarizeMetricsDataRequest

	request.CompartmentId = &compartmentID
	request.SummarizeMetricsDataDetails = details

	return request
}

func (c *Client) collectLatestDatapoint(
	ctx context.Context,
	request monitoring.SummarizeMetricsDataRequest,
) (float32, bool, error) {
	var (
		pageToken       *string
		latestValue     float32
		latestTimestamp time.Time
	)

	found := false

	for {
		response, nextPage, err := c.metrics.SummarizeMetricsData(ctx, request, pageToken)
		if err != nil {
			return 0, false, fmt.Errorf("summarize metrics: %w", err)
		}

		latestTimestamp, latestValue, found = foldMetricStreams(
			response.Items,
			latestTimestamp,
			latestValue,
			found,
		)

		pageToken = normalizePageToken(nextPage)
		if pageToken == nil {
			break
		}
	}

	if !found {
		return 0, false, nil
	}

	return latestValue, true, nil
}

func foldMetricStreams(
	streams []monitoring.MetricData,
	latestTimestamp time.Time,
	latestValue float32,
	found bool,
) (time.Time, float32, bool) {
	for _, stream := range streams {
		for _, datapoint := range stream.AggregatedDatapoints {
			if datapoint.Value == nil || datapoint.Timestamp == nil {
				continue
			}

			timestamp := datapoint.Timestamp.Time
			if !found || timestamp.After(latestTimestamp) {
				latestTimestamp = timestamp
				latestValue = float32(*datapoint.Value)
				found = true
			}
		}
	}

	return latestTimestamp, latestValue, found
}

func normalizePageToken(token *string) *string {
	if token == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*token)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func escapeDimensionValue(value string) string {
	return strings.ReplaceAll(value, "\"", "\\\"")
}
