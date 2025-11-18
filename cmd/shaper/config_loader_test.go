package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/runtimeconfig"
)

func TestLoadRuntimeConfigOrExitReturnsParseCodeOnValidationError(t *testing.T) {
	t.Parallel()

	var deps runDeps

	deps.loadConfig = func(string) (runtimeconfig.Config, error) {
		return runtimeconfig.Config{}, fmt.Errorf("wrap: %w", adapt.ErrInvalidConfig)
	}

	var stderr bytes.Buffer

	_, exitCode, loaded := loadRuntimeConfigOrExit(deps, "", &stderr)
	if loaded {
		t.Fatal("expected loadRuntimeConfigOrExit to report failure")
	}

	if exitCode != exitCodeParseError {
		t.Fatalf("expected parse error exit code %d, got %d", exitCodeParseError, exitCode)
	}

	if output := stderr.String(); !strings.Contains(output, "failed to load configuration") {
		t.Fatalf("expected diagnostic output, got %q", output)
	}
}
