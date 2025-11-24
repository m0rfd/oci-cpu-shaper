package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"go.uber.org/zap"
	"oci-cpu-shaper/internal/buildinfo"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

func assertRunVersionPrints(t *testing.T, args []string, info buildinfo.Info) {
	t.Helper()

	var stdout bytes.Buffer

	deps := defaultRunDeps()
	deps.newLogger = func(string) (*zap.Logger, error) {
		panic("newLogger should not be called when printing version")
	}
	deps.loadConfig = func(string) (runtimeconfig.Config, error) {
		panic("loadConfig should not be called when printing version")
	}
	deps.currentBuildInfo = func() buildinfo.Info {
		return info
	}
	deps.versionWriter = &stdout

	application := newApp(deps)

	_, _, exitCode, ready := application.bootstrap(t.Context(), args, &stdout)
	if ready {
		t.Fatalf("expected bootstrap to stop after printing version, got exit code %d", exitCode)
	}

	if exitCode != exitCodeSuccess {
		t.Fatalf("expected exit code %d, got %d", exitCodeSuccess, exitCode)
	}

	expected := fmt.Sprintf(
		"{Version:%s GitCommit:%s BuildDate:%s}\n",
		info.Version,
		info.GitCommit,
		info.BuildDate,
	)
	if stdout.String() != expected {
		t.Fatalf("expected stdout %q, got %q", expected, stdout.String())
	}
}

func TestRunVersionFlagPrintsBuildInfo(t *testing.T) {
	t.Parallel()

	info := stubBuildInfo("1.2.3", "commit-hash", "2024-06-01")

	assertRunVersionPrints(t, []string{"--version"}, info)
}

func TestRunVersionSubcommandPrintsBuildInfo(t *testing.T) {
	t.Parallel()

	info := stubBuildInfo("0.0.1", "deadbeef", "2024-07-04")

	assertRunVersionPrints(t, []string{"version"}, info)
}

func TestRunReturnsParseErrorExitCode(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	deps := defaultRunDeps()
	deps.currentBuildInfo = func() buildinfo.Info {
		return stubBuildInfo("", "", "")
	}

	application := newApp(deps)

	_, _, exitCode, ready := application.bootstrap(
		t.Context(),
		[]string{"--mode", "invalid"},
		&stderr,
	)
	if ready {
		t.Fatal("expected bootstrap to fail when parsing invalid mode")
	}

	if exitCode != exitCodeParseError {
		t.Fatalf("expected exit code 2 for parse errors, got %d", exitCode)
	}

	if got := stderr.String(); !strings.Contains(got, "unsupported mode") {
		t.Fatalf("expected error message about unsupported mode, got %q", got)
	}
}
