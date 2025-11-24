package main

import (
	"fmt"
	"io"
	"os"
)

func parseOptionsOrPrintVersion(
	deps runDeps,
	args []string,
	stderr io.Writer,
) (options, int, bool) {
	opts, err := parseArgs(args)
	if err != nil {
		var empty options

		return empty, writeError(stderr, err, exitCodeParseError), false
	}

	if opts.showVersion {
		printVersion(deps)

		var empty options

		return empty, exitCodeSuccess, false
	}

	return opts, exitCodeSuccess, true
}

func printVersion(deps runDeps) {
	info := deps.currentBuildInfo()

	writer := deps.versionWriter
	if writer == nil {
		writer = os.Stdout
	}

	_, _ = fmt.Fprintf(writer, "%+v\n", info)
}
