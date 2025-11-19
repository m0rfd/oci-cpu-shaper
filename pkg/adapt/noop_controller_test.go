// Package adapt keeps noop controller tests in this file to guarantee the dry
// path stays a safe escape hatch even when the adaptive loop evolves.
//
//nolint:testpackage,godoclint // Tests need internal helpers and per-file coverage documentation.
package adapt

import (
	"context"
	"testing"
)

func TestNewNoopController(t *testing.T) {
	t.Parallel()

	ctrl := NewNoopController("  noop-mode  ")
	if ctrl.Mode() != "noop-mode" {
		t.Fatalf("unexpected mode: %q", ctrl.Mode())
	}

	err := ctrl.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if ctrl.State() != StateNormal {
		t.Fatalf("expected noop controller state to be normal, got %v", ctrl.State())
	}

	if ctrl.LastError() != nil {
		t.Fatalf("expected noop controller last error to be nil, got %v", ctrl.LastError())
	}

	if ctrl.LastEstimatorError() != nil {
		t.Fatalf(
			"expected noop controller estimator error to be nil, got %v",
			ctrl.LastEstimatorError(),
		)
	}
}

func TestNewNoopControllerDefaultsMode(t *testing.T) {
	t.Parallel()

	ctrl := NewNoopController("   ")
	if ctrl.Mode() != "noop" {
		t.Fatalf("expected noop mode when input is blank, got %q", ctrl.Mode())
	}
}
