package metricsclient

import (
	"context"

	"oci-cpu-shaper/pkg/oci"
)

// Builder constructs MetricsClient instances for the configured compartment and region.
type Builder func(compartmentID, region string) (oci.MetricsClient, error)

type builderKey struct{}

var defaultBuilderFactory = func() Builder { //nolint:gochecknoglobals // swapped in tests to cover fallback resolution.
	return InstancePrincipalBuilder()
}

// WithBuilder stores a MetricsClient builder on the supplied context.
func WithBuilder(ctx context.Context, builder Builder) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	if builder == nil {
		return ctx
	}

	return context.WithValue(ctx, builderKey{}, builder)
}

// FromContext returns the stored MetricsClient builder or falls back to the supplied default.
// When no default is provided, InstancePrincipalBuilder is used.
func FromContext(ctx context.Context, fallback Builder) Builder {
	if ctx != nil {
		if builder, ok := ctx.Value(builderKey{}).(Builder); ok && builder != nil {
			return builder
		}
	}

	if fallback != nil {
		return fallback
	}

	return defaultBuilderFactory()
}
