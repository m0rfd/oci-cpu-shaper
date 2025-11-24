package healthcheck_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"oci-cpu-shaper/pkg/healthcheck"
)

func TestCheckerCheckSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"mode":"rootless","state":"normal"}`))
	}))
	t.Cleanup(server.Close)

	checker, err := healthcheck.NewChecker(server.URL)
	if err != nil {
		t.Fatalf("expected checker, got error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)

	err = checker.Check(ctx)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}

func TestCheckerUnexpectedState(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"mode":"rootless","state":"unknown"}`))
	}))
	t.Cleanup(server.Close)

	checker, err := healthcheck.NewChecker(server.URL)
	if err != nil {
		t.Fatalf("expected checker, got error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)

	err = checker.Check(ctx)
	if err == nil || !strings.Contains(err.Error(), "unhealthy controller state") {
		t.Fatalf("expected unhealthy state error, got %v", err)
	}
}

func TestCheckerExpectedMode(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"mode":"rootless","state":"normal"}`))
	}))
	t.Cleanup(server.Close)

	checker, err := healthcheck.NewChecker(server.URL, healthcheck.WithExpectedMode("rootless"))
	if err != nil {
		t.Fatalf("expected checker, got error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)

	err = checker.Check(ctx)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}

func TestCheckerExpectedModeMismatch(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"mode":"rootless","state":"normal"}`))
	}))
	t.Cleanup(server.Close)

	checker, err := healthcheck.NewChecker(server.URL, healthcheck.WithExpectedMode("rootful"))
	if err != nil {
		t.Fatalf("expected checker, got error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)

	err = checker.Check(ctx)
	if err == nil || !strings.Contains(err.Error(), "unexpected controller mode") {
		t.Fatalf("expected unexpected mode error, got %v", err)
	}
}

func TestCheckerRespectsAllowedStates(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"mode":"rootless","state":"suppressed"}`))
	}))
	t.Cleanup(server.Close)

	checker, err := healthcheck.NewChecker(server.URL, healthcheck.WithAllowedStates("normal"))
	if err != nil {
		t.Fatalf("expected checker, got error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)

	err = checker.Check(ctx)
	if err == nil || !strings.Contains(err.Error(), "unhealthy controller state") {
		t.Fatalf("expected unhealthy state error, got %v", err)
	}
}

func TestCheckerAllowedStatesNormalization(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"mode":"rootless","state":"suppressed"}`))
	}))
	t.Cleanup(server.Close)

	checker, err := healthcheck.NewChecker(
		server.URL,
		healthcheck.WithAllowedStates(" suppressed ", "NORMAL"),
	)
	if err != nil {
		t.Fatalf("expected checker, got error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)

	err = checker.Check(ctx)
	if err != nil {
		t.Fatalf("expected suppressed state to be allowed, got %v", err)
	}
}

func TestCheckerHTTPFailure(t *testing.T) {
	t.Parallel()

	checker, err := healthcheck.NewChecker("http://127.0.0.1:0")
	if err != nil {
		t.Fatalf("expected checker, got error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	t.Cleanup(cancel)

	err = checker.Check(ctx)
	if err == nil || !strings.Contains(err.Error(), "execute health request") {
		t.Fatalf("expected request failure, got %v", err)
	}
}

func TestCheckerStatusCodeHandling(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("maintenance"))
	}))
	t.Cleanup(server.Close)

	checker, err := healthcheck.NewChecker(server.URL)
	if err != nil {
		t.Fatalf("expected checker, got error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)

	err = checker.Check(ctx)
	if err == nil || !strings.Contains(err.Error(), "health endpoint returned non-200 status") {
		t.Fatalf("expected status failure, got %v", err)
	}
}

func TestCheckerInvalidPayload(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	t.Cleanup(server.Close)

	checker, err := healthcheck.NewChecker(server.URL)
	if err != nil {
		t.Fatalf("expected checker, got error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)

	err = checker.Check(ctx)
	if err == nil || !strings.Contains(err.Error(), "decode health payload") {
		t.Fatalf("expected decode failure, got %v", err)
	}
}

func TestNewCheckerRequiresURL(t *testing.T) {
	t.Parallel()

	_, err := healthcheck.NewChecker("   ")
	if !errors.Is(err, healthcheck.ErrMissingURL()) {
		t.Fatalf("expected empty URL error, got %v", err)
	}
}

func TestNewCheckerRequiresClient(t *testing.T) {
	t.Parallel()

	_, err := healthcheck.NewChecker("http://127.0.0.1", healthcheck.WithHTTPClient(nil))
	if !errors.Is(err, healthcheck.ErrMissingClient()) {
		t.Fatalf("expected missing client error, got %v", err)
	}
}
