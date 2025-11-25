package metricsclient

import (
	"context"
	"sync"

	"oci-cpu-shaper/pkg/oci"
)

// Builder constructs MetricsClient instances for the configured compartment and region.
type Builder func(compartmentID, region string) (oci.MetricsClient, error)

type builderKey struct{}

type builderFactory struct {
	mu      sync.RWMutex
	factory func() Builder
}

func newDefaultBuilderFactory() *builderFactory {
	return &builderFactory{ //nolint:exhaustruct // defaults set explicitly below.
		factory: func() Builder {
			return InstancePrincipalBuilder()
		},
	}
}

func (b *builderFactory) resolve() Builder {
	b.mu.RLock()
	factory := b.factory
	b.mu.RUnlock()

	if factory == nil {
		return nil
	}

	return factory()
}

// swap replaces the default factory and returns a restore function for callers.
func (b *builderFactory) swap(factory func() Builder) func() {
	b.mu.Lock()
	previous := b.factory
	b.factory = factory
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		b.factory = previous
		b.mu.Unlock()
	}
}

//nolint:gochecknoglobals // swapped in tests to cover fallback resolution.
var defaultBuilderFactory = newDefaultBuilderFactory()

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

	return defaultBuilderFactory.resolve()
}
