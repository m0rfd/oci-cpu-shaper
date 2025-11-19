package oci

import (
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

// ClientFactory holds constructor hooks for building monitoring clients.
type ClientFactory struct {
	// InstancePrincipalProvider returns the configuration provider used by the OCI SDK.
	InstancePrincipalProvider func() (common.ConfigurationProvider, error)
	// MonitoringClient constructs a Monitoring client using the provided configuration provider.
	MonitoringClient func(common.ConfigurationProvider) (monitoring.MonitoringClient, error)
	// Clock returns the current time and defaults to time.Now.
	Clock func() time.Time
}

// NewClientFactory returns a ClientFactory populated with the SDK defaults used by production code.
func NewClientFactory() *ClientFactory {
	return &ClientFactory{
		InstancePrincipalProvider: auth.InstancePrincipalConfigurationProvider,
		MonitoringClient:          monitoring.NewMonitoringClientWithConfigurationProvider,
		Clock:                     time.Now,
	}
}

type ClientOption func(*clientOptions)

type clientOptions struct {
	factory *ClientFactory
}

// WithFactory injects a ClientFactory into client constructors.
func WithFactory(factory *ClientFactory) ClientOption {
	return func(opts *clientOptions) {
		opts.factory = factory
	}
}

func resolveFactory(options []ClientOption) ClientFactory {
	var opts clientOptions

	for _, opt := range options {
		opt(&opts)
	}

	defaults := NewClientFactory()
	if opts.factory == nil {
		return *defaults
	}

	resolved := ClientFactory{
		InstancePrincipalProvider: defaults.InstancePrincipalProvider,
		MonitoringClient:          defaults.MonitoringClient,
		Clock:                     defaults.Clock,
	}

	if opts.factory.InstancePrincipalProvider != nil {
		resolved.InstancePrincipalProvider = opts.factory.InstancePrincipalProvider
	}

	if opts.factory.MonitoringClient != nil {
		resolved.MonitoringClient = opts.factory.MonitoringClient
	}

	if opts.factory.Clock != nil {
		resolved.Clock = opts.factory.Clock
	}

	return resolved
}
