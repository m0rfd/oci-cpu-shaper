// Package adapt documents default mode handling and enforcement toggles alongside
// the exported configuration surface.
//
//nolint:testpackage,godoclint // Tests rely on private constants for coverage documentation.
package adapt

import "testing"

func TestModeEnforcesTargets(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		mode      string
		enforcing bool
	}{
		{name: "dry-run disabled", mode: dryRunModeLabel, enforcing: false},
		{name: "empty defaults to enforcing", mode: "", enforcing: true},
		{name: "explicit enforce", mode: "enforce", enforcing: true},
		{name: "case insensitive", mode: " EnFoRcE ", enforcing: true},
		{name: "unknown modes enforce", mode: "observe", enforcing: true},
		{name: "noop disables enforcement", mode: " NoOp ", enforcing: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := ModeEnforcesTargets(tc.mode); got != tc.enforcing {
				t.Fatalf("expected enforcing=%v for mode %q, got %v", tc.enforcing, tc.mode, got)
			}
		})
	}
}
