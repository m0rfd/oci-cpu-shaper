//nolint:testpackage // tests require access to unexported config helpers.
package runtimeconfig

import (
	"testing"
	"time"
)

func TestParseFloatDefault(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		fallback float64
		want     float64
	}{
		{name: "empty", input: "   ", fallback: 0.42, want: 0.42},
		{name: "invalid", input: "not-a-number", fallback: 0.5, want: 0.5},
		{name: "valid", input: " 0.33 ", fallback: 1.0, want: 0.33},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := parseFloatDefault(tc.input, tc.fallback); got != tc.want {
				t.Fatalf("parseFloatDefault(%q)=%v want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestEnvDurationFallbacks(t *testing.T) {
	keyInvalid := "OCI_CPU_SHAPER_TEST_DURATION_INVALID"
	t.Setenv(keyInvalid, "invalid")

	if got := envDuration(keyInvalid, 3*time.Second); got != 3*time.Second {
		t.Fatalf("expected invalid duration to use fallback, got %v", got)
	}

	keyBlank := "OCI_CPU_SHAPER_TEST_DURATION_BLANK"
	t.Setenv(keyBlank, "   ")

	if got := envDuration(keyBlank, 2*time.Second); got != 2*time.Second {
		t.Fatalf("expected blank duration to use fallback, got %v", got)
	}

	keyValid := "OCI_CPU_SHAPER_TEST_DURATION_VALID"
	t.Setenv(keyValid, "150ms")

	if got := envDuration(keyValid, time.Second); got != 150*time.Millisecond {
		t.Fatalf("expected valid duration 150ms, got %v", got)
	}
}

func TestEnvIntRejectsNonPositive(t *testing.T) {
	keyNegative := "OCI_CPU_SHAPER_TEST_INT_NEGATIVE"
	t.Setenv(keyNegative, "-5")

	if got := envInt(keyNegative, 7); got != 7 {
		t.Fatalf("expected negative int fallback 7, got %d", got)
	}

	keyZero := "OCI_CPU_SHAPER_TEST_INT_ZERO"
	t.Setenv(keyZero, "0")

	if got := envInt(keyZero, 4); got != 4 {
		t.Fatalf("expected zero fallback 4, got %d", got)
	}

	keyValid := "OCI_CPU_SHAPER_TEST_INT_VALID"
	t.Setenv(keyValid, " 5 ")

	if got := envInt(keyValid, 1); got != 5 {
		t.Fatalf("expected trimmed int 5, got %d", got)
	}

	keyInvalid := "OCI_CPU_SHAPER_TEST_INT_INVALID"
	t.Setenv(keyInvalid, "not-a-number")

	if got := envInt(keyInvalid, 9); got != 9 {
		t.Fatalf("expected invalid int to fall back to 9, got %d", got)
	}
}

func TestEnvStringTrimsAndFallback(t *testing.T) {
	keyBlank := "OCI_CPU_SHAPER_TEST_STRING_BLANK"
	t.Setenv(keyBlank, "   ")

	if got := envString(keyBlank, "fallback"); got != "fallback" {
		t.Fatalf("expected blank string fallback, got %q", got)
	}

	keyValue := "OCI_CPU_SHAPER_TEST_STRING_VALUE"
	t.Setenv(keyValue, "  value  ")

	if got := envString(keyValue, "fallback"); got != "value" {
		t.Fatalf("expected trimmed value, got %q", got)
	}
}

func TestEnvStringAllowEmpty(t *testing.T) {
	keyMissing := "OCI_CPU_SHAPER_TEST_STRING_ALLOW_EMPTY_MISSING"
	if got := envStringAllowEmpty(keyMissing, "fallback"); got != "fallback" {
		t.Fatalf("expected missing env to use fallback, got %q", got)
	}

	keyBlank := "OCI_CPU_SHAPER_TEST_STRING_ALLOW_EMPTY_BLANK"
	t.Setenv(keyBlank, "  ")

	if got := envStringAllowEmpty(keyBlank, "fallback"); got != "" {
		t.Fatalf("expected blank string override to produce empty value, got %q", got)
	}

	keyValue := "OCI_CPU_SHAPER_TEST_STRING_ALLOW_EMPTY_VALUE"
	t.Setenv(keyValue, " custom ")

	if got := envStringAllowEmpty(keyValue, "fallback"); got != "custom" {
		t.Fatalf("expected trimmed override, got %q", got)
	}
}

func TestEnvBoolEvaluation(t *testing.T) {
	if got := envBool("OCI_CPU_SHAPER_TEST_BOOL_MISSING", true); got != true {
		t.Fatalf("expected missing env bool to return fallback, got %t", got)
	}

	keyTrue := "OCI_CPU_SHAPER_TEST_BOOL_TRUE"
	t.Setenv(keyTrue, "Yes")

	if got := envBool(keyTrue, false); !got {
		t.Fatal("expected affirmative string to parse as true")
	}

	keyFalse := "OCI_CPU_SHAPER_TEST_BOOL_FALSE"
	t.Setenv(keyFalse, "0")

	if got := envBool(keyFalse, true); got {
		t.Fatal("expected zero to parse as false")
	}

	keyInvalid := "OCI_CPU_SHAPER_TEST_BOOL_INVALID"
	t.Setenv(keyInvalid, "sometimes")

	if got := envBool(keyInvalid, false); got {
		t.Fatal("expected invalid bool to fall back to false")
	}
}
