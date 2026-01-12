package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"mode":"rootless","state":"normal"}`))
	}))
	t.Cleanup(server.Close)

	err := runWithArgs([]string{"--url", server.URL, "--timeout", "1s"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}

func TestRunUnexpectedState(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"mode":"rootless","state":"unknown"}`))
	}))
	t.Cleanup(server.Close)

	err := runWithArgs([]string{"--url", server.URL, "--allowed-states", "normal"})
	if err == nil || !strings.Contains(err.Error(), "unhealthy controller state") {
		t.Fatalf("expected unhealthy state error, got %v", err)
	}
}

func TestSplitStates(t *testing.T) {
	t.Parallel()

	states := splitStates("normal, fallback ,  ,suppressed")
	if len(states) != 3 {
		t.Fatalf("expected 3 states, got %d", len(states))
	}
}

func TestRunTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond)

		_, _ = w.Write([]byte(`{"mode":"rootless","state":"normal"}`))
	}))
	t.Cleanup(server.Close)

	err := runWithArgs([]string{"--url", server.URL, "--timeout", "50ms"})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestRunRejectsInvalidURL(t *testing.T) {
	t.Parallel()

	err := runWithArgs([]string{"--url", "http://%zz"})
	if err == nil {
		t.Fatal("expected url parsing error, got nil")
	}
}
