package main

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"oci-cpu-shaper/pkg/adapt"
)

func TestHandleControllerRunResultLogsCompletion(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	code := handleControllerRunResult(logger, nil)
	if code != exitCodeSuccess {
		t.Fatalf("expected success exit code, got %d", code)
	}

	entries := observed.FilterMessage("controller stopped").All()
	if len(entries) != 1 {
		t.Fatalf("expected controller stopped log entry, got %+v", observed.All())
	}

	requireLogFieldString(t, entries[0], "reason", "completed")
}

func TestExitCodeForConfigError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "invalid config",
			err:  adapt.ErrInvalidConfig,
			want: exitCodeParseError,
		},
		{
			name: "runtime error",
			err:  errStubControllerRun,
			want: exitCodeRuntimeError,
		},
		{
			name: "nil error",
			err:  nil,
			want: exitCodeRuntimeError,
		},
	}

	for _, tc := range testCases {
		testCase := tc

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := exitCodeForConfigError(testCase.err); got != testCase.want {
				t.Fatalf("expected %d, got %d", testCase.want, got)
			}
		})
	}
}
