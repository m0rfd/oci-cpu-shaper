package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"
)

type app struct {
	deps runDeps
}

func newApp(deps runDeps) app {
	return app{deps: deps}
}

func (a app) Run(ctx context.Context, args []string, stderr io.Writer) int {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-sigCh:
			cancel()
		}
	}()

	return a.run(ctx, args, stderr)
}
