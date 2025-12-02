//nolint:testpackage // tests require access to unexported config helpers.
package runtimeconfig

import (
	"testing"
	"time"
)

type envIntCase struct {
	name     string
	key      string
	value    string
	setEnv   bool
	fallback int
	expected int
}

func runEnvIntTestCases(t *testing.T, parser func(string, int) int, cases []envIntCase) {
	t.Helper()

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.setEnv {
				t.Setenv(testCase.key, testCase.value)
			}

			if got := parser(testCase.key, testCase.fallback); got != testCase.expected {
				t.Fatalf(
					"unexpected value for %s: got %d, want %d",
					testCase.key,
					got,
					testCase.expected,
				)
			}
		})
	}
}

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

//nolint:paralleltest // uses t.Setenv which cannot run in parallel.
func TestEnvIntRejectsNonPositive(t *testing.T) {
	defaultEnvIntCases := []envIntCase{
		{
			name:     "missing_uses_fallback",
			key:      "OCI_CPU_SHAPER_TEST_INT_MISSING",
			value:    "",
			setEnv:   false,
			fallback: 11,
			expected: 11,
		},
		{
			name:     "blank_uses_fallback",
			key:      "OCI_CPU_SHAPER_TEST_INT_BLANK",
			value:    "   ",
			setEnv:   true,
			fallback: 12,
			expected: 12,
		},
		{
			name:     "non_numeric",
			key:      "OCI_CPU_SHAPER_TEST_INT_NON_NUMERIC",
			value:    "invalid",
			setEnv:   true,
			fallback: 9,
			expected: 9,
		},
		{
			name:     "negative",
			key:      "OCI_CPU_SHAPER_TEST_INT_NEGATIVE",
			value:    "-5",
			setEnv:   true,
			fallback: 7,
			expected: 7,
		},
		{
			name:     "zero",
			key:      "OCI_CPU_SHAPER_TEST_INT_ZERO",
			value:    "0",
			setEnv:   true,
			fallback: 4,
			expected: 4,
		},
		{
			name:     "valid_high_positive",
			key:      "OCI_CPU_SHAPER_TEST_INT_VALID",
			value:    " 123456 ",
			setEnv:   true,
			fallback: 1,
			expected: 123456,
		},
	}

	runEnvIntTestCases(t, envInt, defaultEnvIntCases)
}

//nolint:paralleltest // uses t.Setenv which cannot run in parallel.
func TestEnvIntAllowZero(t *testing.T) {
	envIntAllowZeroCases := []envIntCase{
		{
			name:     "missing_uses_fallback",
			key:      "OCI_CPU_SHAPER_TEST_INT_ALLOW_ZERO_MISSING",
			value:    "",
			setEnv:   false,
			fallback: 13,
			expected: 13,
		},
		{
			name:     "blank_uses_fallback",
			key:      "OCI_CPU_SHAPER_TEST_INT_ALLOW_ZERO_BLANK",
			value:    "  ",
			setEnv:   true,
			fallback: 8,
			expected: 8,
		},
		{
			name:     "non_numeric",
			key:      "OCI_CPU_SHAPER_TEST_INT_ALLOW_ZERO_NON_NUMERIC",
			value:    "oops",
			setEnv:   true,
			fallback: 6,
			expected: 6,
		},
		{
			name:     "negative",
			key:      "OCI_CPU_SHAPER_TEST_INT_ALLOW_ZERO_NEGATIVE",
			value:    "-2",
			setEnv:   true,
			fallback: 4,
			expected: 4,
		},
		{
			name:     "zero_accepted",
			key:      "OCI_CPU_SHAPER_TEST_INT_ALLOW_ZERO",
			value:    "0",
			setEnv:   true,
			fallback: 3,
			expected: 0,
		},
		{
			name:     "valid_high_positive",
			key:      "OCI_CPU_SHAPER_TEST_INT_ALLOW_ZERO_POSITIVE",
			value:    "98765",
			setEnv:   true,
			fallback: 2,
			expected: 98765,
		},
	}

	runEnvIntTestCases(t, envIntAllowZero, envIntAllowZeroCases)
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

//nolint:paralleltest // uses t.Setenv which cannot run in parallel.
func TestApplyEnvOverridesIntegratesIntParsers(t *testing.T) {
	defaultCfg := Default()
	expected := defaultCfg
	expected.Controller.RelaxedConfirmations = 15
	expected.Controller.SuppressSmoothingSamples = 0

	overrides := map[string]string{
		envPoolWorkers:              "-8",
		envRelaxedConfirmations:     "15",
		envSuppressSmoothingSamples: "0",
	}

	for key, value := range overrides {
		t.Setenv(key, value)
	}

	cfg := defaultCfg
	applyEnvOverrides(&cfg)

	if cfg != expected {
		t.Fatalf("expected %#v after env overrides, got %#v", expected, cfg)
	}
}

//nolint:paralleltest // uses t.Setenv which cannot run in parallel.
func TestApplyEnvOverridesInvalidNumbersFallbackToDefaults(t *testing.T) {
	assertEnvOverridesKeepDefaults(t, map[string]string{
		envTargetStart:               "   ",
		envTargetMin:                 "invalid-min",
		envTargetMax:                 "oops",
		envStepUp:                    " ",
		envStepDown:                  "invalid",
		envFallbackTarget:            "not-a-number",
		envGoalLow:                   "",
		envGoalHigh:                  "  ",
		envRelaxedThreshold:          "bad",
		envSuppressThreshold:         "not-a-number",
		envSuppressResume:            "none",
		envSuppressRunnableThreshold: "oops",
		envSuppressRunnableResume:    "??",
		envSuppressSmoothingSamples:  "nah",
		envPoolPauseThreshold:        "oops",
		envPoolResumeThreshold:       "resume-not-number",
		// OCI offline should ignore unknown values and keep the fallback.
		envOCIOffline: "sometimes",
	})
}

//nolint:paralleltest // uses t.Setenv which cannot run in parallel.
func TestApplyEnvOverridesRejectNonPositiveWorkerCount(t *testing.T) {
	assertEnvOverridesKeepDefaults(t, map[string]string{envPoolWorkers: "-4"})
	assertEnvOverridesKeepDefaults(t, map[string]string{envPoolWorkers: "0"})
}

//nolint:paralleltest // uses t.Setenv which cannot run in parallel.
func TestApplyEnvOverridesRejectMalformedDurations(t *testing.T) {
	assertEnvOverridesKeepDefaults(t, map[string]string{
		envSlowInterval:    "   ",
		envRelaxedInterval: "1xs",
		envFastInterval:    "duration?",
	})
}

func assertEnvOverridesKeepDefaults(t *testing.T, overrides map[string]string) {
	t.Helper()

	defaultCfg := Default()
	cfg := defaultCfg

	for key, value := range overrides {
		t.Setenv(key, value)
	}

	applyEnvOverrides(&cfg)

	if cfg != defaultCfg {
		t.Fatalf("expected config to retain defaults with overrides %#v, got %#v", overrides, cfg)
	}
}
