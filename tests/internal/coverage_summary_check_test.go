package internal_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverageSummaryCheckFailsWithoutTotal(t *testing.T) {
	t.Parallel()

	summaryPath := filepath.Join(t.TempDir(), "coverage.txt")

	err := os.WriteFile(summaryPath, []byte("mode: atomic\n"), 0o600)
	if err != nil {
		t.Fatalf("failed to write coverage summary: %v", err)
	}

	output, err := runCoverageSummaryCheck(t, summaryPath, "0")
	if err == nil {
		t.Fatalf("expected coverage summary check to fail without a total, got success: %s", output)
	}

	if !strings.Contains(string(output), "missing a total coverage entry") {
		t.Fatalf("expected missing total coverage message, got: %s", output)
	}
}

func TestCoverageSummaryCheckPassesWithValidTotal(t *testing.T) {
	t.Parallel()

	summaryPath := filepath.Join(t.TempDir(), "coverage.txt")

	contents := "pkg/example\t90.0%\ntotal:\t(statements)\t99.5%\n"

	err := os.WriteFile(summaryPath, []byte(contents), 0o600)
	if err != nil {
		t.Fatalf("failed to write coverage summary: %v", err)
	}

	output, err := runCoverageSummaryCheck(t, summaryPath, "95")
	if err != nil {
		t.Fatalf("expected coverage summary check to pass, got error: %v output: %s", err, output)
	}
}

func runCoverageSummaryCheck(t *testing.T, summaryPath, minCoverage string) ([]byte, error) {
	t.Helper()

	script := filepath.Join(repoRoot(t), "hack", "coverage_summary_check.sh")
	cmd := exec.CommandContext(context.Background(), "bash", script, summaryPath, minCoverage)
	cmd.Env = append(os.Environ(), "MIN_COVERAGE="+minCoverage)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("running coverage summary check: %w", err)
	}

	return output, nil
}

func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("failed to resolve repository root: %v", err)
	}

	return root
}
