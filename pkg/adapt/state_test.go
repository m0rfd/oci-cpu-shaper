package adapt_test

import (
	"testing"

	"oci-cpu-shaper/pkg/adapt"
)

func TestStateString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		state    adapt.State
		expected string
	}{
		{
			name:     "normal",
			state:    adapt.StateNormal,
			expected: "normal",
		},
		{
			name:     "fallback",
			state:    adapt.StateFallback,
			expected: "fallback",
		},
		{
			name:     "suppressed",
			state:    adapt.StateSuppressed,
			expected: "suppressed",
		},
		{
			name:     "unknown",
			state:    adapt.State(42),
			expected: "unknown",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.state.String(); got != testCase.expected {
				t.Fatalf("expected %q, got %q", testCase.expected, got)
			}
		})
	}
}
