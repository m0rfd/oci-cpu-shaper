package oci

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

const (
	monitoringNamespace = "oci_computeagent"
	// OCI Monitoring does not support `.window()` for SummarizeMetricsData, so we fetch one-minute
	// 95th percentile CPU utilization samples over the 7-day range and compute the percentile across
	// the full window locally.
	metricQueryTemplate     = "CpuUtilization[1m]{resourceId = \"%s\"}.percentile(0.95)"
	metricName              = "CpuUtilization"
	maxOneMinuteWindowHours = 7 * 24
	percentileTarget        = 0.95
)

// QueryP95CPU returns the trailing seven-day P95 CpuUtilization percentile for the supplied compute instance.
// The query spans the trailing seven days at one-minute resolution to match the reclaim horizon and
// the Monitoring API's resolution limit. ErrNoMetricsData is returned when the API yields no
// datapoints.
func (c *Client) QueryP95CPU(
	ctx context.Context,
	instanceOCID string,
) (float64, time.Time, error) {
	if c == nil {
		return 0, time.Time{}, errNilClient
	}

	if instanceOCID == "" {
		return 0, time.Time{}, errMissingInstanceOCID
	}

	start, end := computeWindow(c.now().UTC())
	request := buildSummarizeRequest(c.compartmentID, instanceOCID, start, end)

	value, fetchedAt, found, err := c.collectLatestDatapoint(ctx, request)
	if err != nil {
		return 0, time.Time{}, err
	}

	if !found {
		return 0, time.Time{}, ErrNoMetricsData
	}

	return value, fetchedAt, nil
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

// collectLatestDatapoint pages through the SummarizeMetricsData responses and computes
// the requested percentile across the full window. It returns false when the API yields
// no datapoints.
func (c *Client) collectLatestDatapoint(
	ctx context.Context,
	request monitoring.SummarizeMetricsDataRequest,
) (float64, time.Time, bool, error) {
	pageToken := (*string)(nil)
	values := make([]float64, 0)
	latest := time.Time{}

	for {
		response, nextPage, err := c.metrics.SummarizeMetricsData(ctx, request, pageToken)
		if err != nil {
			return 0, time.Time{}, false, fmt.Errorf("summarize metrics: %w", err)
		}

		values, latest = appendMetricValues(values, latest, response.Items)

		pageToken = normalizePageToken(nextPage)
		if pageToken == nil {
			break
		}
	}

	if len(values) == 0 {
		return 0, time.Time{}, false, nil
	}

	percentile := percentile(values, percentileTarget)

	return percentile, latest, true, nil
}

func appendMetricValues(
	values []float64,
	latest time.Time,
	streams []monitoring.MetricData,
) ([]float64, time.Time) {
	for _, stream := range streams {
		for _, datapoint := range stream.AggregatedDatapoints {
			if datapoint.Value == nil || datapoint.Timestamp == nil {
				continue
			}

			values = append(values, *datapoint.Value)

			timestamp := datapoint.Timestamp.Time
			if timestamp.After(latest) {
				latest = timestamp
			}
		}
	}

	return values, latest
}

func percentile(values []float64, target float64) float64 {
	sort.Float64s(values)

	rank := max(int(math.Ceil(target*float64(len(values)))), 1)

	index := rank - 1
	if index >= len(values) {
		index = len(values) - 1
	}

	return values[index]
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
