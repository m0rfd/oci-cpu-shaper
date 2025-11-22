//go:build integration

package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/cgroup"
	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

type integrationBlockingController struct {
	mode string
}

func (c *integrationBlockingController) Run(ctx context.Context) error {
	<-ctx.Done()

	return ctx.Err()
}

func (c *integrationBlockingController) Mode() string { return c.mode }

func (c *integrationBlockingController) State() adapt.State { return adapt.StateNormal }

func (c *integrationBlockingController) LastError() error { return nil }

func (c *integrationBlockingController) LastEstimatorError() error { return nil }

func TestMetricsServerReportsUnavailableCgroupOnDetectionFailure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	core, observed := observer.New(zap.InfoLevel)

	serverCh := make(chan *httptest.Server, 1)

	runner := NewIntegrationRunner()
	runner.WithLoggerFactory(func(string) (*zap.Logger, error) { return zap.New(core), nil })
	runner.WithConfigLoader(func(string) (runtimeconfig.Config, error) {
		cfg := runtimeconfig.Default()
		cfg.HTTP.Bind = freeTCPAddress(t)
		cfg.OCI.Offline = true

		return cfg, nil
	})
	runner.WithStartMetricsServer(func(ctx context.Context, _ *zap.Logger, _ string, handler http.Handler) (func(context.Context), error) {
		server := newIPv4TestServer(t, handler)
		serverCh <- server

		go func() {
			<-ctx.Done()
			server.Close()
		}()

		return func(context.Context) { server.Close() }, nil
	})
	runner.WithDetectCgroup(func() (*cgroup.CPU, error) { return nil, errors.New("integration cgroup failure") })
	runner.WithControllerFactory(func(
		ctx context.Context,
		mode string,
		cfg runtimeconfig.Config,
		imdsClient imds.Client,
		recorder adapt.MetricsRecorder,
	) (adapt.Controller, PoolStarter, error) {
		_ = cfg
		_ = imdsClient
		_ = recorder

		return &integrationBlockingController{mode: mode}, nil, nil
	})

	exitCh := make(chan int, 1)
	go func() {
		exitCh <- runner.Run(ctx, []string{"-mode", "noop"}, io.Discard)
	}()

	var server *httptest.Server
	select {
	case server = <-serverCh:
	case <-time.After(time.Second):
		t.Fatalf("metrics server did not start in time")
	}

	metricsCtx, metricsCancel := context.WithTimeout(ctx, time.Second)
	defer metricsCancel()

	metricsBody := waitForMetrics(t, metricsCtx, server.URL+"/metrics")

	metrics := string(metricsBody)
	for _, expected := range []string{
		"cgroup_cpu_weight 0",
		"cgroup_cpu_max_quota 0",
		"cgroup_cpu_max_period 0",
		"cgroup_cpu_max_unlimited 0",
	} {
		if !strings.Contains(metrics, expected) {
			t.Fatalf("expected metrics to contain %q, got:\n%s", expected, metrics)
		}
	}

	warnings := observed.FilterMessage("failed to inspect cgroup cpu settings").All()
	if len(warnings) != 1 {
		t.Fatalf("expected one cgroup detection warning, got %d entries", len(warnings))
	}

	cancel()

	select {
	case code := <-exitCh:
		if code != 0 {
			t.Fatalf("expected zero exit code, got %d", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("run did not exit after cancel")
	}
}

func waitForMetrics(t *testing.T, ctx context.Context, url string) []byte {
	t.Helper()

	client := http.Client{ //nolint:exhaustruct // timeout configured
		Timeout: time.Second,
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("scrape metrics: %v", ctx.Err())
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
			if err != nil {
				t.Fatalf("scrape metrics: build request: %v", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				continue
			}

			body, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				_ = resp.Body.Close()

				t.Fatalf("scrape metrics: read body: %v", readErr)
			}

			closeErr := resp.Body.Close()
			if closeErr != nil {
				t.Fatalf("scrape metrics: close body: %v", closeErr)
			}

			if resp.StatusCode != http.StatusOK {
				continue
			}

			if len(body) == 0 {
				continue
			}

			return body
		}
	}
}
