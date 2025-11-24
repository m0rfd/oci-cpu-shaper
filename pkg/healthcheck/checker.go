package healthcheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	maxErrorBodyBytes  int64 = 1024
	defaultHTTPTimeout       = 5 * time.Second
)

// HTTPClient describes the subset of http.Client used by the checker.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Snapshot mirrors the health payload served by the controller.
type Snapshot struct {
	Mode           string `json:"mode"`
	State          string `json:"state"`
	LastOCIError   string `json:"ociError"`
	EstimatorError string `json:"estimatorError"`
}

// Checker validates the health endpoint response.
type Checker struct {
	client        HTTPClient
	url           string
	allowedStates map[string]struct{}
	expectedMode  string
}

// Option mutates a Checker instance.
type Option func(*Checker)

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(client HTTPClient) Option {
	return func(c *Checker) {
		c.client = client
	}
}

// WithAllowedStates configures the list of states treated as healthy.
func WithAllowedStates(states ...string) Option {
	return func(c *Checker) {
		c.allowedStates = normalizeSet(states)
	}
}

// WithExpectedMode enforces a specific controller mode when provided.
func WithExpectedMode(mode string) Option {
	desired := normalizeToken(mode)

	return func(c *Checker) {
		c.expectedMode = desired
	}
}

var (
	errMissingURL      = errors.New("health endpoint URL is required")
	errMissingClient   = errors.New("HTTP client is required")
	errUninitialised   = errors.New("health checker is not initialised")
	errStatusUnhealthy = errors.New("health endpoint returned non-200 status")
	errMissingState    = errors.New("health payload missing state")
	errMissingMode     = errors.New("health payload missing mode")
	errUnhealthyState  = errors.New("unhealthy controller state")
	errUnexpectedMode  = errors.New("unexpected controller mode")
)

// DefaultAllowedStates returns the controller states treated as healthy when no override is provided.
func DefaultAllowedStates() []string {
	return []string{"normal", "fallback", "suppressed"}
}

// NewChecker constructs a health checker.
func NewChecker(url string, opts ...Option) (*Checker, error) {
	trimmedURL := strings.TrimSpace(url)
	if trimmedURL == "" {
		return nil, errMissingURL
	}

	checker := &Checker{
		url:           trimmedURL,
		allowedStates: copySet(defaultAllowedStates()),
		expectedMode:  "",
		client: &http.Client{ //nolint:exhaustruct // only timeout differs in consumers
			Timeout: defaultHTTPTimeout,
		},
	}

	for _, opt := range opts {
		opt(checker)
	}

	if checker.client == nil {
		return nil, errMissingClient
	}

	return checker, nil
}

// Check performs a single health check request.
func (c *Checker) Check(ctx context.Context) error {
	if c == nil {
		return errUninitialised
	}

	resp, err := c.doRequest(ctx)
	if err != nil {
		return err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	statusErr := evaluateStatus(resp)
	if statusErr != nil {
		return statusErr
	}

	snapshot, err := decodeSnapshot(resp.Body)
	if err != nil {
		return err
	}

	return c.validateSnapshot(snapshot)
}

func (c *Checker) doRequest(ctx context.Context) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build health request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute health request: %w", err)
	}

	return resp, nil
}

func evaluateStatus(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}

	payload, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))

	body := strings.TrimSpace(string(payload))
	if body != "" {
		return fmt.Errorf("%w: %d %s", errStatusUnhealthy, resp.StatusCode, body)
	}

	return fmt.Errorf("%w: %d", errStatusUnhealthy, resp.StatusCode)
}

func decodeSnapshot(reader io.Reader) (Snapshot, error) {
	decoder := json.NewDecoder(reader)

	var snapshot Snapshot

	decodeErr := decoder.Decode(&snapshot)
	if decodeErr != nil {
		return Snapshot{}, fmt.Errorf("decode health payload: %w", decodeErr)
	}

	return snapshot, nil
}

func (c *Checker) validateSnapshot(snapshot Snapshot) error {
	state := normalizeToken(snapshot.State)
	if state == "" {
		return errMissingState
	}

	if len(c.allowedStates) > 0 {
		if _, ok := c.allowedStates[state]; !ok {
			return fmt.Errorf("%w: %s", errUnhealthyState, state)
		}
	}

	if c.expectedMode == "" {
		return nil
	}

	mode := normalizeToken(snapshot.Mode)
	if mode == "" {
		return errMissingMode
	}

	if mode != c.expectedMode {
		return fmt.Errorf("%w: %s (want %s)", errUnexpectedMode, mode, c.expectedMode)
	}

	return nil
}

func normalizeSet(values []string) map[string]struct{} {
	cleaned := make(map[string]struct{}, len(values))
	for _, value := range values {
		token := normalizeToken(value)
		if token != "" {
			cleaned[token] = struct{}{}
		}
	}

	return cleaned
}

func copySet(values map[string]struct{}) map[string]struct{} {
	if len(values) == 0 {
		return map[string]struct{}{}
	}

	clone := make(map[string]struct{}, len(values))
	for value := range values {
		clone[value] = struct{}{}
	}

	return clone
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func defaultAllowedStates() map[string]struct{} {
	return normalizeSet(DefaultAllowedStates())
}

// ErrMissingURL exposes the sentinel used when the health URL is empty.
func ErrMissingURL() error {
	return errMissingURL
}

// ErrMissingClient exposes the sentinel used when the HTTP client is unset.
func ErrMissingClient() error {
	return errMissingClient
}
