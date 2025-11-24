package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRunReturnsHealthyOnSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"state":"normal"}`))
	}))
	t.Cleanup(server.Close)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	exitCode := run([]string{"-addr", server.URL, "-timeout", "2s"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%s)", exitCode, stderr.String())
	}

	if body := stdout.String(); body != "healthy\n" {
		t.Fatalf("unexpected stdout: %q", body)
	}
}

func TestRunFailsOnUnhealthyState(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"state":"fallback"}`))
	}))
	t.Cleanup(server.Close)

	exitCode := run(
		[]string{"-addr", server.URL, "-ok-states", "normal"},
		new(bytes.Buffer),
		new(bytes.Buffer),
	)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
}

func TestRunRejectsBadFlag(t *testing.T) {
	t.Parallel()

	exitCode := run([]string{"-timeout", "five"}, new(bytes.Buffer), new(bytes.Buffer))
	if exitCode != 2 {
		t.Fatalf("expected exit code 2 on flag parse error, got %d", exitCode)
	}
}

func TestSplitStates(t *testing.T) {
	t.Parallel()

	normalized := splitStates(" normal, ,suppressed ,,fallback")

	if len(normalized) != 3 {
		t.Fatalf("expected three entries, got %d", len(normalized))
	}

	expected := map[string]struct{}{"normal": {}, "suppressed": {}, "fallback": {}}
	for _, state := range normalized {
		if _, ok := expected[state]; !ok {
			t.Fatalf("unexpected state %q", state)
		}
	}
}

func TestRunHonorsTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-time.After(250 * time.Millisecond)

		_, _ = w.Write([]byte(`{"state":"normal"}`))
	}))
	t.Cleanup(server.Close)

	exitCode := run(
		[]string{"-addr", server.URL, "-timeout", "50ms"},
		new(bytes.Buffer),
		new(bytes.Buffer),
	)
	if exitCode != 1 {
		t.Fatalf("expected timeout failure, got %d", exitCode)
	}
}

func TestRunNormalizesTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"state":"normal"}`))
	}))
	t.Cleanup(server.Close)

	negativeTimeout := "-1s"

	exitCode := run(
		[]string{"-addr", server.URL, "-timeout", negativeTimeout},
		new(bytes.Buffer),
		new(bytes.Buffer),
	)
	if exitCode != 0 {
		t.Fatalf("expected negative timeout to be normalized, got exit %d", exitCode)
	}
}
