package imds //nolint:testpackage

import (
	"net/http"
	"testing"
	"time"
)

func TestWithBaseURL(t *testing.T) {
	t.Parallel()

	t.Run("trims and sets", func(t *testing.T) {
		t.Parallel()

		cfg := clientConfig{
			baseURL:    DefaultEndpoint,
			maxAttempt: defaultMaxAttempts,
			backoff:    defaultBackoff,
		}

		WithBaseURL("  http://example.com/opc/v2  ")(&cfg)

		if cfg.baseURL != "http://example.com/opc/v2" {
			t.Fatalf("baseURL = %q, want %q", cfg.baseURL, "http://example.com/opc/v2")
		}
	})

	t.Run("ignores empty input", func(t *testing.T) {
		t.Parallel()

		cfg := clientConfig{
			baseURL:    DefaultEndpoint,
			maxAttempt: defaultMaxAttempts,
			backoff:    defaultBackoff,
		}

		WithBaseURL("")(&cfg)
		WithBaseURL("   ")(&cfg)

		if cfg.baseURL != DefaultEndpoint {
			t.Fatalf("baseURL = %q, want %q", cfg.baseURL, DefaultEndpoint)
		}
	})
}

func TestWithMaxAttempts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    int
		expected int
	}{
		{name: "positive overrides", input: 5, expected: 5},
		{name: "zero ignored", input: 0, expected: defaultMaxAttempts},
		{name: "negative ignored", input: -1, expected: defaultMaxAttempts},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := clientConfig{
				baseURL:    DefaultEndpoint,
				maxAttempt: defaultMaxAttempts,
				backoff:    defaultBackoff,
			}

			WithMaxAttempts(testCase.input)(&cfg)

			if cfg.maxAttempt != testCase.expected {
				t.Fatalf("maxAttempt = %d, want %d", cfg.maxAttempt, testCase.expected)
			}
		})
	}
}

func TestWithBackoff(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    time.Duration
		expected time.Duration
	}{
		{name: "positive overrides", input: 50 * time.Millisecond, expected: 50 * time.Millisecond},
		{name: "zero ignored", input: 0, expected: defaultBackoff},
		{name: "negative ignored", input: -time.Second, expected: defaultBackoff},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := clientConfig{
				baseURL:    DefaultEndpoint,
				maxAttempt: defaultMaxAttempts,
				backoff:    defaultBackoff,
			}

			WithBackoff(testCase.input)(&cfg)

			if cfg.backoff != testCase.expected {
				t.Fatalf("backoff = %s, want %s", cfg.backoff, testCase.expected)
			}
		})
	}
}

func TestNewClientUsesDefaultsWhenHTTPClientNil(t *testing.T) {
	t.Parallel()

	client := NewClient(nil, nil, WithBaseURL("  "+DefaultEndpoint+"/  "))

	if client.http == nil {
		t.Fatalf("http client is nil")
	}

	if client.http.Timeout != defaultHTTPClientTimeout {
		t.Fatalf("Timeout = %s, want %s", client.http.Timeout, defaultHTTPClientTimeout)
	}

	if client.baseURL != DefaultEndpoint {
		t.Fatalf("baseURL = %q, want %q", client.baseURL, DefaultEndpoint)
	}

	if client.maxAttempt != defaultMaxAttempts {
		t.Fatalf("maxAttempt = %d, want %d", client.maxAttempt, defaultMaxAttempts)
	}

	if client.backoff != defaultBackoff {
		t.Fatalf("backoff = %s, want %s", client.backoff, defaultBackoff)
	}
}

func TestNewClientSkipsNilOptions(t *testing.T) {
	t.Parallel()

	httpClient := new(http.Client)

	client := NewClient(httpClient, nil, WithBackoff(100*time.Millisecond))

	if client.http != httpClient {
		t.Fatalf("http client changed")
	}

	if client.backoff != 100*time.Millisecond {
		t.Fatalf("backoff = %s, want %s", client.backoff, 100*time.Millisecond)
	}
}
