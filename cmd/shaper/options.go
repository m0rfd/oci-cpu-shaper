package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

type options struct {
	configPath    string
	logLevel      string
	mode          string
	shutdownAfter time.Duration
	showVersion   bool
}

var (
	errInvalidLogLevel      = errors.New("invalid log level")
	errUnsupportedMode      = errors.New("unsupported mode provided")
	errInvalidShutdownAfter = errors.New("invalid shutdown-after duration (must be >=0)")
)

func parseArgs(args []string) (options, error) {
	var opts options

	flagSet := flag.NewFlagSet("shaper", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	flagSet.BoolVar(
		&opts.showVersion,
		"version",
		false,
		"Print build information and exit",
	)
	flagSet.StringVar(
		&opts.configPath,
		"config",
		defaultConfigPath,
		"Path to the shaper configuration file",
	)
	flagSet.StringVar(
		&opts.logLevel,
		"log-level",
		defaultLogLevel,
		"Structured log level (debug, info, warn, error)",
	)
	flagSet.StringVar(
		&opts.mode,
		"mode",
		modeEnforce,
		"Controller mode to use (dry-run, enforce, noop)",
	)
	flagSet.DurationVar(
		&opts.shutdownAfter,
		"shutdown-after",
		0,
		"Gracefully stop the controller after the provided duration (0 disables the timer)",
	)

	err := flagSet.Parse(args)
	if err != nil {
		return options{}, fmt.Errorf("parse CLI arguments: %w", err)
	}

	if !opts.showVersion {
		if slices.Contains(flagSet.Args(), "version") {
			opts.showVersion = true
		}
	}

	if opts.showVersion {
		return opts, nil
	}

	normErr := normalizeOptions(&opts)
	if normErr != nil {
		return options{}, normErr
	}

	return opts, nil
}

func normalizeOptions(opts *options) error {
	if opts == nil {
		return nil
	}

	opts.mode = strings.ToLower(strings.TrimSpace(opts.mode))
	if opts.mode == "" {
		opts.mode = modeEnforce
	}

	if !isValidMode(opts.mode) {
		return fmt.Errorf(
			"%w: %q (supported: %s, %s, %s)",
			errUnsupportedMode,
			opts.mode,
			modeDryRun,
			modeEnforce,
			modeNoop,
		)
	}

	opts.logLevel = strings.TrimSpace(opts.logLevel)
	if opts.logLevel == "" {
		opts.logLevel = defaultLogLevel
	}

	opts.configPath = strings.TrimSpace(opts.configPath)
	if opts.configPath == "" {
		opts.configPath = defaultConfigPath
	}

	if opts.shutdownAfter < 0 {
		return fmt.Errorf("%w: %v", errInvalidShutdownAfter, opts.shutdownAfter)
	}

	return nil
}

func isValidMode(mode string) bool {
	switch mode {
	case modeDryRun, modeEnforce, modeNoop:
		return true
	default:
		return false
	}
}
