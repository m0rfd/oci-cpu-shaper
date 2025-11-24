package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"oci-cpu-shaper/internal/buildinfo"
)

func TestParseOptionsOrPrintVersionHandlesErrors(t *testing.T) {
	t.Parallel()

	t.Run("parse failure", func(t *testing.T) {
		t.Parallel()

		deps := defaultRunDeps()

		opts, exitCode, proceed := parseOptionsOrPrintVersion(
			deps,
			[]string{"--mode", "invalid"},
			io.Discard,
		)
		if proceed {
			t.Fatalf("expected parseOptionsOrPrintVersion to halt on parse error: %#v", opts)
		}

		if exitCode != exitCodeParseError {
			t.Fatalf("expected parse error exit code %d, got %d", exitCodeParseError, exitCode)
		}
	})

	t.Run("version", func(t *testing.T) {
		t.Parallel()

		var output bytes.Buffer

		deps := defaultRunDeps()
		deps.versionWriter = &output
		deps.currentBuildInfo = func() buildinfo.Info {
			return buildinfo.Info{Version: "vTest", GitCommit: "deadbeef", BuildDate: "today"}
		}

		opts, exitCode, proceed := parseOptionsOrPrintVersion(
			deps,
			[]string{"--version"},
			io.Discard,
		)
		if proceed {
			t.Fatalf("expected parseOptionsOrPrintVersion to stop on version flag: %#v", opts)
		}

		if exitCode != exitCodeSuccess {
			t.Fatalf("expected success exit code, got %d", exitCode)
		}

		if !strings.Contains(output.String(), "Version:vTest") ||
			!strings.Contains(output.String(), "GitCommit:deadbeef") {
			t.Fatalf("unexpected version output: %q", output.String())
		}
	})
}

//nolint:paralleltest // captures os.Stdout; running in parallel would affect other tests.
func TestParseOptionsOrPrintVersionUsesStdoutWhenVersionWriterUnset(t *testing.T) {
	var (
		opts     options
		exitCode int
		proceed  bool
	)

	output := captureStdout(t, func() {
		deps := defaultRunDeps()
		deps.versionWriter = nil
		deps.currentBuildInfo = func() buildinfo.Info {
			return buildinfo.Info{Version: "vTest", GitCommit: "deadbeef", BuildDate: "today"}
		}

		opts, exitCode, proceed = parseOptionsOrPrintVersion(
			deps,
			[]string{"--version"},
			io.Discard,
		)
	})

	if proceed {
		t.Fatalf("expected parseOptionsOrPrintVersion to halt when printing version: %#v", opts)
	}

	if exitCode != exitCodeSuccess {
		t.Fatalf("expected success exit code, got %d", exitCode)
	}

	if !strings.Contains(output, "Version:vTest") ||
		!strings.Contains(output, "GitCommit:deadbeef") {
		t.Fatalf("unexpected version output to stdout: %q", output)
	}
}

func captureStdout(t *testing.T, captureFn func()) string {
	t.Helper()

	originalStdout := os.Stdout

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}

	t.Cleanup(func() {
		os.Stdout = originalStdout
	})

	t.Cleanup(func() {
		_ = reader.Close()
	})

	t.Cleanup(func() {
		_ = writer.Close()
	})

	os.Stdout = writer

	captureFn()

	writerCloseErr := writer.Close()
	if writerCloseErr != nil {
		t.Fatalf("failed to close stdout writer: %v", writerCloseErr)
	}

	var output bytes.Buffer

	_, copyErr := io.Copy(&output, reader)
	if copyErr != nil {
		t.Fatalf("failed to capture stdout output: %v", copyErr)
	}

	return output.String()
}

func TestParseOptionsOrPrintVersionReturnsOptions(t *testing.T) {
	t.Parallel()

	deps := defaultRunDeps()

	opts, exitCode, proceed := parseOptionsOrPrintVersion(
		deps,
		[]string{"--mode", modeDryRun},
		io.Discard,
	)
	if !proceed {
		t.Fatal("expected parseOptionsOrPrintVersion to proceed when flags parse")
	}

	if exitCode != exitCodeSuccess {
		t.Fatalf("expected exit code %d, got %d", exitCodeSuccess, exitCode)
	}

	if opts.mode != modeDryRun {
		t.Fatalf("expected mode %q, got %q", modeDryRun, opts.mode)
	}
}
