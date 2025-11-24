package healthcheck_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/healthcheck"
)

func TestCheckerAcceptsHealthyState(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		snapshot := healthcheck.Snapshot{
			Mode:           "dry-run",
			State:          "normal",
			LastOCIError:   "",
			EstimatorError: "",
		}
		encodeSnapshot(t, w, snapshot)
	}))
	t.Cleanup(server.Close)

	checkerConfig := healthcheck.Config{
		Endpoint:        server.URL,
		Timeout:         time.Second,
		HealthyStates:   nil,
		RequireNoErrors: true,
	}

	checker, err := healthcheck.NewChecker(checkerConfig)
	if err != nil {
		t.Fatalf("build checker: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	err = checker.Check(ctx)
	if err != nil {
		t.Fatalf("unexpected health failure: %v", err)
	}
}

func TestCheckerRejectsFallbackState(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		encodeSnapshot(
			t,
			w,
			healthcheck.Snapshot{Mode: "", State: "fallback", LastOCIError: "", EstimatorError: ""},
		)
	}))
	t.Cleanup(server.Close)

	checkerConfig := healthcheck.Config{
		Endpoint:        server.URL,
		Timeout:         time.Second,
		HealthyStates:   []string{"normal"},
		RequireNoErrors: true,
	}

	checker, err := healthcheck.NewChecker(checkerConfig)
	if err != nil {
		t.Fatalf("build checker: %v", err)
	}

	err = checker.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "fallback") {
		t.Fatalf("expected fallback state failure, got %v", err)
	}
}

func TestCheckerRejectsReportedErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		encodeSnapshot(
			t,
			w,
			healthcheck.Snapshot{
				Mode:           "",
				State:          "normal",
				LastOCIError:   "dial tcp",
				EstimatorError: "",
			},
		)
	}))
	t.Cleanup(server.Close)

	checkerConfig := healthcheck.Config{
		Endpoint:        server.URL,
		Timeout:         time.Second,
		HealthyStates:   nil,
		RequireNoErrors: true,
	}

	checker, err := healthcheck.NewChecker(checkerConfig)
	if err != nil {
		t.Fatalf("build checker: %v", err)
	}

	err = checker.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "dial tcp") {
		t.Fatalf("expected error propagation, got %v", err)
	}
}

func TestCheckerAllowsErrorsWhenDisabled(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		encodeSnapshot(
			t,
			w,
			healthcheck.Snapshot{
				Mode:           "",
				State:          "suppressed",
				LastOCIError:   "",
				EstimatorError: "lag",
			},
		)
	}))
	t.Cleanup(server.Close)

	checkerConfig := healthcheck.Config{
		Endpoint:        server.URL,
		Timeout:         time.Second,
		HealthyStates:   nil,
		RequireNoErrors: false,
	}

	checker, err := healthcheck.NewChecker(checkerConfig)
	if err != nil {
		t.Fatalf("build checker: %v", err)
	}

	err = checker.Check(context.Background())
	if err != nil {
		t.Fatalf("expected healthcheck to ignore errors, got %v", err)
	}
}

func TestCheckerHandlesNon200Response(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	checkerConfig := healthcheck.Config{
		Endpoint:        server.URL,
		Timeout:         time.Second,
		HealthyStates:   nil,
		RequireNoErrors: true,
	}

	checker, err := healthcheck.NewChecker(checkerConfig)
	if err != nil {
		t.Fatalf("build checker: %v", err)
	}

	err = checker.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func encodeSnapshot(t *testing.T, w http.ResponseWriter, snapshot healthcheck.Snapshot) {
	t.Helper()

	encoder := json.NewEncoder(w)
	err := encoder.Encode(snapshot)
	if err != nil {
		t.Fatalf("encode snapshot: %v", err)
	}
}
