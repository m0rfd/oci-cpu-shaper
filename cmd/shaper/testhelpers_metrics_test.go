package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	stubDefaultP95CPU = 0.25
	metricsServerWait = time.Second
)

var (
	errMetricsServerBoom = errors.New("metrics server start failure")
	errStubQueryFailure  = errors.New("stub: query failure")
)

type stubMetricsAdapter struct{}

func newStubMetricsClient() *stubMetricsAdapter { return &stubMetricsAdapter{} }

func (*stubMetricsAdapter) QueryP95CPU(context.Context, string) (float64, time.Time, error) {
	return stubDefaultP95CPU, time.Time{}, nil
}

func expectMetricsSnippets(t *testing.T, output string, snippets []string) {
	t.Helper()

	for _, snippet := range snippets {
		if !strings.Contains(output, snippet) {
			t.Fatalf("expected metrics output to contain %q, got:\n%s", snippet, output)
		}
	}
}

type healthSnapshot struct {
	State          string `json:"state"`
	Mode           string `json:"mode"`
	LastOCIError   string `json:"ociError"`
	EstimatorError string `json:"estimatorError"`
}

func fetchHealthSnapshot(
	ctx context.Context,
	t *testing.T,
	client *http.Client,
	baseURL string,
) healthSnapshot {
	t.Helper()

	request, buildErr := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
	if buildErr != nil {
		t.Fatalf("build health request: %v", buildErr)
	}

	response, doErr := client.Do(request)
	if doErr != nil {
		t.Fatalf("GET /healthz failed: %v", doErr)
	}

	defer func() {
		closeErr := response.Body.Close()
		if closeErr != nil {
			t.Fatalf("close health response body: %v", closeErr)
		}
	}()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /healthz, got %d", response.StatusCode)
	}

	decoder := json.NewDecoder(response.Body)

	var snapshot healthSnapshot

	decodeErr := decoder.Decode(&snapshot)
	if decodeErr != nil {
		t.Fatalf("decode health response: %v", decodeErr)
	}

	return snapshot
}
