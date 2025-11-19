package main

import (
	"errors"
	"flag"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseArgsDefaults(t *testing.T) {
	t.Parallel()

	opts, err := parseArgs(nil)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if opts.configPath != defaultConfigPath {
		t.Fatalf("expected default config path, got %q", opts.configPath)
	}

	if opts.logLevel != defaultLogLevel {
		t.Fatalf("expected default log level, got %q", opts.logLevel)
	}

	if opts.mode != modeDryRun {
		t.Fatalf("expected default mode, got %q", opts.mode)
	}

	if opts.shutdownAfter != 0 {
		t.Fatalf("expected shutdownAfter default to be 0, got %v", opts.shutdownAfter)
	}
}

func TestParseArgsValidCustomizations(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join("..", "pkg", "runtimeconfig", "testdata", "config.yaml")

	args := []string{
		"--config",
		configPath,
		"--log-level",
		"debug",
		"--mode",
		"enforce",
		"--shutdown-after",
		"45s",
	}

	opts, err := parseArgs(args)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if opts.configPath != configPath {
		t.Fatalf("unexpected config path: %q", opts.configPath)
	}

	if opts.logLevel != "debug" {
		t.Fatalf("unexpected log level: %q", opts.logLevel)
	}

	if opts.mode != modeEnforce {
		t.Fatalf("unexpected mode: %q", opts.mode)
	}

	if opts.shutdownAfter != 45*time.Second {
		t.Fatalf("unexpected shutdownAfter: %v", opts.shutdownAfter)
	}
}

func TestParseArgsRejectsNegativeShutdownAfter(t *testing.T) {
	t.Parallel()

	_, err := parseArgs([]string{"--shutdown-after", "-5s"})
	if err == nil {
		t.Fatal("expected error for negative shutdown-after duration")
	}

	if !errors.Is(err, errInvalidShutdownAfter) {
		t.Fatalf("expected errInvalidShutdownAfter, got %v", err)
	}
}

func TestParseArgsRejectsUnknownMode(t *testing.T) {
	t.Parallel()

	_, err := parseArgs([]string{"--mode", "observe"})
	if err == nil {
		t.Fatal("expected error for unsupported mode")
	}
}

func TestParseArgsTrimSpaces(t *testing.T) {
	t.Parallel()

	opts, err := parseArgs([]string{"--mode", "  NOOP ", "--log-level", " info "})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if opts.mode != modeNoop {
		t.Fatalf("expected trimmed lowercase mode, got %q", opts.mode)
	}

	if opts.logLevel != defaultLogLevel {
		t.Fatalf("expected trimmed log level, got %q", opts.logLevel)
	}
}

func TestParseArgsVersionFlag(t *testing.T) {
	t.Parallel()

	opts, err := parseArgs([]string{"--version"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if !opts.showVersion {
		t.Fatal("expected showVersion to be true when --version is provided")
	}
}

func TestParseArgsVersionSubcommand(t *testing.T) {
	t.Parallel()

	opts, err := parseArgs([]string{"version"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if !opts.showVersion {
		t.Fatal("expected showVersion to be true when version subcommand is provided")
	}
}

func TestParseArgsReturnsFlagError(t *testing.T) {
	t.Parallel()

	_, err := parseArgs([]string{"--unknown-flag"})
	if err == nil {
		t.Fatal("expected flag parsing error")
	}

	if !errors.Is(err, flag.ErrHelp) &&
		!strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("unexpected error type: %v", err)
	}
}

func TestNormalizeOptionsNilOptions(t *testing.T) {
	t.Parallel()

	err := normalizeOptions(nil)
	if err != nil {
		t.Fatalf("normalizeOptions returned error for nil options: %v", err)
	}
}

func TestNormalizeOptionsTrimsAndDefaults(t *testing.T) {
	t.Parallel()

	opts := &options{
		configPath:    " ",
		logLevel:      "  ",
		mode:          "  ENFORCE  ",
		shutdownAfter: 0,
		showVersion:   false,
	}

	err := normalizeOptions(opts)
	if err != nil {
		t.Fatalf("normalizeOptions returned error: %v", err)
	}

	if opts.mode != modeEnforce {
		t.Fatalf("expected normalized mode %q, got %q", modeEnforce, opts.mode)
	}

	if opts.logLevel != defaultLogLevel {
		t.Fatalf("expected default log level %q, got %q", defaultLogLevel, opts.logLevel)
	}

	if opts.configPath != defaultConfigPath {
		t.Fatalf("expected default config path %q, got %q", defaultConfigPath, opts.configPath)
	}
}

func TestNormalizeOptionsRejectsUnsupportedMode(t *testing.T) {
	t.Parallel()

	opts := &options{
		configPath:    defaultConfigPath,
		logLevel:      defaultLogLevel,
		mode:          "invalid",
		shutdownAfter: 0,
		showVersion:   false,
	}

	err := normalizeOptions(opts)
	if err == nil {
		t.Fatal("expected error for unsupported mode")
	}

	if !errors.Is(err, errUnsupportedMode) {
		t.Fatalf("expected errUnsupportedMode, got %v", err)
	}
}

func TestNormalizeOptionsRejectsNegativeShutdown(t *testing.T) {
	t.Parallel()

	opts := &options{
		configPath:    defaultConfigPath,
		logLevel:      defaultLogLevel,
		mode:          modeDryRun,
		shutdownAfter: -time.Second,
		showVersion:   false,
	}

	err := normalizeOptions(opts)
	if err == nil {
		t.Fatal("expected error for negative shutdown")
	}

	if !errors.Is(err, errInvalidShutdownAfter) {
		t.Fatalf("expected errInvalidShutdownAfter, got %v", err)
	}
}

func TestNormalizeOptionsAppliesDefaultMode(t *testing.T) {
	t.Parallel()

	opts := &options{
		configPath:    defaultConfigPath,
		logLevel:      defaultLogLevel,
		mode:          "   ",
		shutdownAfter: 0,
		showVersion:   false,
	}

	err := normalizeOptions(opts)
	if err != nil {
		t.Fatalf("normalizeOptions returned error: %v", err)
	}

	if opts.mode != modeDryRun {
		t.Fatalf("expected default mode %q, got %q", modeDryRun, opts.mode)
	}
}
