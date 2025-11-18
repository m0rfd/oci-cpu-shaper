//nolint:testpackage // tests require access to unexported config helpers.
package runtimeconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/adapt"
)

const testConfigFilePerm = 0o600

func adaptDefault() adapt.Config {
	return adapt.DefaultConfig()
}

func writeTempConfig(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	err := os.WriteFile(path, []byte(contents), testConfigFilePerm)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	return path
}

func assertInvalidRuntimeConfigError(t *testing.T, err error, substr string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected configuration validation error")
	}

	if !errors.Is(err, adapt.ErrInvalidConfig) {
		t.Fatalf("expected adapt.ErrInvalidConfig, got %v", err)
	}

	if substr != "" && !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error to reference %q, got %v", substr, err)
	}
}

func assertFloatEqual(t *testing.T, name string, got, want float64) {
	t.Helper()

	if got != want {
		t.Fatalf("expected %s override %v, got %v", name, want, got)
	}
}

func assertDurationEqual(t *testing.T, name string, got, want time.Duration) {
	t.Helper()

	if got != want {
		t.Fatalf("expected %s override %v, got %v", name, want, got)
	}
}

func assertIntEqual(t *testing.T, name string, got, want int) {
	t.Helper()

	if got != want {
		t.Fatalf("expected %s override %d, got %d", name, want, got)
	}
}

func assertStringEqual(t *testing.T, name, got, want string) {
	t.Helper()

	if got != want {
		t.Fatalf("expected %s override %q, got %q", name, want, got)
	}
}

func assertBoolEqual(t *testing.T, name string, got, want bool) {
	t.Helper()

	if got != want {
		t.Fatalf("expected %s override %t, got %t", name, want, got)
	}
}
