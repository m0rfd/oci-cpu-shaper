package main

import (
	"sync"

	"oci-cpu-shaper/pkg/oci/metricsclient"
)

type (
	instancePrincipalBuilderFactory func(...metricsclient.Option) metricsclient.Builder
	instancePrincipalBuilderState   struct {
		factory instancePrincipalBuilderFactory
		mu      sync.RWMutex
	}
)

// newInstancePrincipalBuilder allows tests to inject a custom metrics builder.
//
//nolint:gochecknoglobals // swapped in tests to cover wiring.
var newInstancePrincipalBuilder = instancePrincipalBuilderState{
	factory: metricsclient.InstancePrincipalBuilder,
	mu:      sync.RWMutex{},
}

func (state *instancePrincipalBuilderState) load() instancePrincipalBuilderFactory {
	state.mu.RLock()
	factory := state.factory
	state.mu.RUnlock()

	return factory
}

func (state *instancePrincipalBuilderState) swap(
	factory instancePrincipalBuilderFactory,
) instancePrincipalBuilderFactory {
	state.mu.Lock()
	previous := state.factory
	state.factory = factory
	state.mu.Unlock()

	return previous
}
