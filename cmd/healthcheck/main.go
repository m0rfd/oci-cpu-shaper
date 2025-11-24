package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"oci-cpu-shaper/pkg/healthcheck"
)

const (
	defaultHealthURL   = "http://127.0.0.1:9108/healthz"
	defaultTimeout     = 5 * time.Second
	allowedStatesUsage = "comma-separated controller states treated as healthy"
)

func main() {
	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	return runWithArgs(os.Args[1:])
}

func runWithArgs(args []string) error {
	flagSet := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	defaultStates := strings.Join(healthcheck.DefaultAllowedStates(), ",")

	healthURL := flagSet.String(
		"url",
		defaultHealthURL,
		"HTTP endpoint exposing the /healthz payload",
	)
	allowedStates := flagSet.String("allowed-states", defaultStates, allowedStatesUsage)
	expectedMode := flagSet.String(
		"expected-mode",
		"",
		"optional controller mode to require (e.g. rootless)",
	)
	timeout := flagSet.Duration("timeout", defaultTimeout, "overall check timeout")

	parseErr := flagSet.Parse(args)
	if parseErr != nil {
		return fmt.Errorf("parse flags: %w", parseErr)
	}

	parsedURL, err := url.ParseRequestURI(strings.TrimSpace(*healthURL))
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}

	stateList := splitStates(*allowedStates)
	client := &http.Client{ //nolint:exhaustruct // only timeout differs
		Timeout: *timeout,
	}

	checker, err := healthcheck.NewChecker(parsedURL.String(),
		healthcheck.WithAllowedStates(stateList...),
		healthcheck.WithExpectedMode(*expectedMode),
		healthcheck.WithHTTPClient(client),
	)
	if err != nil {
		return fmt.Errorf("configure checker: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	checkErr := checker.Check(ctx)
	if checkErr != nil {
		return fmt.Errorf("poll health endpoint: %w", checkErr)
	}

	_, printErr := fmt.Fprintln(os.Stdout, "ok")
	if printErr != nil {
		return fmt.Errorf("write success response: %w", printErr)
	}

	return nil
}

func splitStates(value string) []string {
	parts := strings.Split(value, ",")
	cleaned := make([]string, 0, len(parts))

	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token != "" {
			cleaned = append(cleaned, token)
		}
	}

	return cleaned
}
