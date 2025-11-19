package adapt

import (
	"context"
	"strings"
)

// NoopController satisfies the Controller interface but performs no work.
type NoopController struct {
	mode string
}

var _ Controller = (*NoopController)(nil)

// NewNoopController builds a controller that immediately returns without work.
func NewNoopController(mode string) *NoopController {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" {
		trimmed = noopModeLabel
	}

	return &NoopController{mode: trimmed}
}

// Run implements the Controller interface.
func (n *NoopController) Run(context.Context) error { return nil }

// Mode implements the Controller interface.
func (n *NoopController) Mode() string { return n.mode }

// State implements the Controller interface.
func (n *NoopController) State() State { return StateNormal }

// LastError implements the Controller interface.
func (n *NoopController) LastError() error { return nil }

// LastEstimatorError implements the Controller interface.
func (n *NoopController) LastEstimatorError() error { return nil }
