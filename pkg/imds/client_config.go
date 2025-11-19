package imds

import (
	"net/http"
	"strings"
	"time"
)

const (
	defaultHTTPClientTimeout = 2 * time.Second
	defaultMaxAttempts       = 3
	defaultBackoff           = 200 * time.Millisecond
)

type clientConfig struct {
	baseURL    string
	maxAttempt int
	backoff    time.Duration
}

// Option mutates the HTTP client configuration during construction.
type Option func(*clientConfig)

// WithBaseURL overrides the metadata service base URL used for requests.
func WithBaseURL(baseURL string) Option {
	return func(cfg *clientConfig) {
		trimmed := strings.TrimSpace(baseURL)
		if trimmed == "" {
			return
		}

		cfg.baseURL = trimmed
	}
}

// WithMaxAttempts overrides the retry budget for metadata requests.
func WithMaxAttempts(attempts int) Option {
	return func(cfg *clientConfig) {
		if attempts > 0 {
			cfg.maxAttempt = attempts
		}
	}
}

// WithBackoff overrides the delay between retry attempts.
func WithBackoff(delay time.Duration) Option {
	return func(cfg *clientConfig) {
		if delay > 0 {
			cfg.backoff = delay
		}
	}
}

// HTTPClient issues metadata requests against the OCI IMDSv2 service.
type HTTPClient struct {
	http       *http.Client
	baseURL    string
	maxAttempt int
	backoff    time.Duration
}

// NewClient constructs an HTTP-backed IMDS client. A nil httpClient uses a
// private instance with a conservative timeout suitable for link-local access.
//
//nolint:ireturn // controller wiring depends on returning the Client interface for swapping transports in tests.
func NewClient(httpClient *http.Client, opts ...Option) Client {
	cfg := clientConfig{
		baseURL:    DefaultEndpoint,
		maxAttempt: defaultMaxAttempts,
		backoff:    defaultBackoff,
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}

		opt(&cfg)
	}

	if httpClient == nil {
		httpClient = &http.Client{
			Timeout:       defaultHTTPClientTimeout,
			Transport:     http.DefaultTransport,
			CheckRedirect: http.DefaultClient.CheckRedirect,
			Jar:           http.DefaultClient.Jar,
		}
	}

	return &HTTPClient{
		http:       httpClient,
		baseURL:    strings.TrimRight(cfg.baseURL, "/"),
		maxAttempt: cfg.maxAttempt,
		backoff:    cfg.backoff,
	}
}
