package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"oci-cpu-shaper/pkg/healthcheck"
)

const (
	defaultProbeTimeout = 5 * time.Second
	exitCodeHealthy     = 0
	exitCodeUnhealthy   = 1
	exitCodeBadFlags    = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flagSet := flag.NewFlagSet("oci-cpu-shaper-healthcheck", flag.ContinueOnError)
	flagSet.SetOutput(stderr)

	endpoint := flagSet.String(
		"addr",
		"",
		"health endpoint to query (default http://127.0.0.1:9108/healthz)",
	)
	timeout := flagSet.Duration("timeout", defaultProbeTimeout, "probe timeout")
	healthyStates := flagSet.String(
		"ok-states",
		"normal,suppressed",
		"comma-separated healthy controller states",
	)
	allowErrors := flagSet.Bool(
		"allow-errors",
		false,
		"allow controller error strings without failing the probe",
	)

	parseErr := flagSet.Parse(args)
	if parseErr != nil {
		return exitCodeBadFlags
	}

	timeoutValue := *timeout
	if timeoutValue <= 0 {
		timeoutValue = defaultProbeTimeout
	}

	checker, err := healthcheck.NewChecker(healthcheck.Config{
		Endpoint:        *endpoint,
		Timeout:         timeoutValue,
		HealthyStates:   splitStates(*healthyStates),
		RequireNoErrors: !*allowErrors,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)

		return exitCodeBadFlags
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeoutValue)
	defer cancel()

	err = checker.Check(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)

		return exitCodeUnhealthy
	}

	_, _ = fmt.Fprintln(stdout, "healthy")

	return exitCodeHealthy
}

func splitStates(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	trimmed := make([]string, 0, len(parts))

	for _, part := range parts {
		piece := strings.TrimSpace(part)
		if piece == "" {
			continue
		}

		trimmed = append(trimmed, piece)
	}

	return trimmed
}
