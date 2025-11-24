package main

import (
	"context"
	"io"
)

func (a app) run(
	ctx context.Context,
	args []string,
	stderr io.Writer,
) int {
	ctx, boot, exitCode, ready := a.bootstrap(ctx, args, stderr)
	if !ready {
		return exitCode
	}
	defer boot.cleanup()

	runtime, exitCode, controllerReady := a.prepareController(ctx, boot)
	if !controllerReady {
		return exitCode
	}
	defer runtime.cleanup(ctx)

	return runtime.start(ctx)
}

func run(ctx context.Context, args []string, deps runDeps, stderr io.Writer) int {
	return newApp(deps).run(ctx, args, stderr)
}
