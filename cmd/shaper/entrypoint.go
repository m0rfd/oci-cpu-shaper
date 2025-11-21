// Package main wires the shaper CLI entrypoint.
package main

import (
	"context"
	"os"
)

func main() {
	code := newApp(defaultRunDeps()).Run(context.Background(), os.Args[1:], os.Stderr)
	if code != 0 {
		exitProcess(code)
	}
}

var exitProcess = os.Exit //nolint:gochecknoglobals // replaceable for tests
