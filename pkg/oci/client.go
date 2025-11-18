// Package oci hosts helpers for interacting with Oracle Cloud Infrastructure APIs.
package oci

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

var (
	// ErrNoMetricsData indicates that the Monitoring service returned no datapoints for the
	// requested CpuUtilization stream. Callers may fall back to local estimation logic when this
	// sentinel error is returned.
	ErrNoMetricsData = errors.New("oci: cpu utilization metrics unavailable")

	errMissingCompartmentID = errors.New("oci: compartment ID is required")
	errMissingMetricsClient = errors.New("oci: metrics client is required")
	errNilClient            = errors.New("oci: metrics client receiver is nil")
	errMissingInstanceOCID  = errors.New("oci: instance OCID is required")
)

// ClientFactory captures the seams required to build a Client.
type ClientFactory struct {
	InstancePrincipalProvider func() (common.ConfigurationProvider, error)
	NewMonitoringClient       func(common.ConfigurationProvider) (monitoring.MonitoringClient, error)
	Now                       func() time.Time
}

// NewClientFactory returns a ClientFactory configured with the default OCI SDK helpers.
func NewClientFactory() ClientFactory {
	return ClientFactory{
		InstancePrincipalProvider: auth.InstancePrincipalConfigurationProvider,
		NewMonitoringClient:       monitoring.NewMonitoringClientWithConfigurationProvider,
		Now:                       time.Now,
	}
}

// WithFactory overrides the factory used by NewInstancePrincipalClient.
func WithFactory(factory ClientFactory) ClientOption {
	return func(options *clientOptions) {
		if options == nil {
			return
		}

		options.factory = factory
	}
}

type ClientOption func(*clientOptions)

type clientOptions struct {
	factory ClientFactory
}

func defaultClientOptions() clientOptions {
	return clientOptions{factory: NewClientFactory()}
}

func applyClientOptions(opts []ClientOption) clientOptions {
	options := defaultClientOptions()

	for _, opt := range opts {
		if opt == nil {
			continue
		}

		opt(&options)
	}

	return options
}

func (f ClientFactory) withDefaults() ClientFactory {
	if f.InstancePrincipalProvider == nil {
		f.InstancePrincipalProvider = auth.InstancePrincipalConfigurationProvider
	}

	if f.NewMonitoringClient == nil {
		f.NewMonitoringClient = monitoring.NewMonitoringClientWithConfigurationProvider
	}

	if f.Now == nil {
		f.Now = time.Now
	}

	return f
}

type metricsClient interface {
	SummarizeMetricsData(
		ctx context.Context,
		request monitoring.SummarizeMetricsDataRequest,
		page *string,
	) (monitoring.SummarizeMetricsDataResponse, *string, error)
}

// Client queries tenancy-level Monitoring metrics for the local instance.
type Client struct {
	metrics       metricsClient
	compartmentID string
	now           func() time.Time
}

// NewInstancePrincipalClient constructs a Client backed by the OCI Go SDK using instance principal
// authentication. The compartment OCID identifies the tenancy scope for Monitoring queries.
func NewInstancePrincipalClient(
	compartmentID, region string,
	opts ...ClientOption,
) (*Client, error) {
	if compartmentID == "" {
		return nil, errMissingCompartmentID
	}

	options := applyClientOptions(opts)
	factory := options.factory.withDefaults()

	provider, err := factory.InstancePrincipalProvider()
	if err != nil {
		return nil, fmt.Errorf("build instance principal provider: %w", err)
	}

	monitoringClient, err := factory.NewMonitoringClient(provider)
	if err != nil {
		return nil, fmt.Errorf("create monitoring client: %w", err)
	}

	trimmedRegion := strings.TrimSpace(region)
	if trimmedRegion != "" {
		monitoringClient.SetRegion(trimmedRegion)
	}

	return newClient(&sdkMonitoringClient{client: &monitoringClient}, compartmentID, factory.Now)
}

func newClient(
	metrics metricsClient,
	compartmentID string,
	clock func() time.Time,
) (*Client, error) {
	if metrics == nil {
		return nil, errMissingMetricsClient
	}

	if compartmentID == "" {
		return nil, errMissingCompartmentID
	}

	if clock == nil {
		clock = time.Now
	}

	return &Client{
		metrics:       metrics,
		compartmentID: compartmentID,
		now:           clock,
	}, nil
}

// newTestClient exposes constructor hooks for unit tests.
func newTestClient(
	metrics metricsClient,
	compartmentID string,
	clock func() time.Time,
) (*Client, error) {
	return newClient(metrics, compartmentID, clock)
}
