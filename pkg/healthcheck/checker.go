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
	defaultEndpoint = "http://127.0.0.1:9108/healthz"
	normalState     = "normal"
	suppressedState = "suppressed"
	defaultTimeout  = 5 * time.Second
)

var (
	// ErrCheckerNotConfigured signals the checker was not initialised.
	ErrCheckerNotConfigured = errors.New("health checker is not configured")
	errUnexpectedStatus     = errors.New("health endpoint returned unexpected status")
	errUnhealthyState       = errors.New("unhealthy controller state")
	errReportedErrors       = errors.New("health endpoint reported errors")
)

// Config configures the health check client.
type Config struct {
	Endpoint        string
	Timeout         time.Duration
	HealthyStates   []string
	RequireNoErrors bool
}

// Checker runs HTTP probes against the shaper health endpoint.
type Checker struct {
	client          *http.Client
	endpoint        string
	healthyStates   map[string]struct{}
	requireNoErrors bool
}

// Snapshot matches the subset of the /healthz response needed for validation.
type Snapshot struct {
	Mode           string `json:"mode"`
	State          string `json:"state"`
	LastOCIError   string `json:"ociError"`
	EstimatorError string `json:"estimatorError"`
}

// NewChecker builds a Checker from the supplied configuration.
func NewChecker(cfg Config) (*Checker, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	healthyStates := normalizeStates(cfg.HealthyStates)
	if len(healthyStates) == 0 {
		healthyStates = map[string]struct{}{normalState: {}, suppressedState: {}}
	}

	return &Checker{
		client: &http.Client{
			Timeout:       timeout,
			Transport:     http.DefaultTransport,
			CheckRedirect: http.DefaultClient.CheckRedirect,
			Jar:           http.DefaultClient.Jar,
		},
		endpoint:        endpoint,
		healthyStates:   healthyStates,
		requireNoErrors: cfg.RequireNoErrors,
	}, nil
}

// Check performs a single probe and returns an error when the endpoint is unhealthy.
func (c *Checker) Check(ctx context.Context) error {
	if c == nil {
		return ErrCheckerNotConfigured
	}

	response, err := c.doRequest(ctx)
	if err != nil {
		return err
	}

	defer func() {
		_ = response.Body.Close()
	}()

	snapshot, err := decodeSnapshot(response.Body)
	if err != nil {
		return err
	}

	err = c.validateState(snapshot.State)
	if err != nil {
		return err
	}

	if !c.requireNoErrors {
		return nil
	}

	return validateErrors(snapshot)
}

func (c *Checker) doRequest(ctx context.Context) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		//nolint:wrapcheck // top-level surface for healthcheck binary
		return nil, err
	}

	response, err := c.client.Do(request)
	if err != nil {
		//nolint:wrapcheck // top-level surface for healthcheck binary
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", errUnexpectedStatus, response.StatusCode)
	}

	return response, nil
}

func decodeSnapshot(reader io.Reader) (Snapshot, error) {
	payload, err := io.ReadAll(reader)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read health response: %w", err)
	}

	var snapshot Snapshot

	err = json.Unmarshal(payload, &snapshot)
	if err != nil {
		return Snapshot{}, fmt.Errorf("decode health response: %w", err)
	}

	return snapshot, nil
}

func (c *Checker) validateState(state string) error {
	trimmed := strings.ToLower(strings.TrimSpace(state))
	if _, ok := c.healthyStates[trimmed]; ok {
		return nil
	}

	return fmt.Errorf("%w: %s", errUnhealthyState, state)
}

func validateErrors(snapshot Snapshot) error {
	var reportedErrors []string
	if trimmed := strings.TrimSpace(snapshot.LastOCIError); trimmed != "" {
		reportedErrors = append(reportedErrors, "ociError="+trimmed)
	}

	if trimmed := strings.TrimSpace(snapshot.EstimatorError); trimmed != "" {
		reportedErrors = append(reportedErrors, "estimatorError="+trimmed)
	}

	if len(reportedErrors) == 0 {
		return nil
	}

	return fmt.Errorf("%w: %s", errReportedErrors, strings.Join(reportedErrors, ", "))
}

func normalizeStates(states []string) map[string]struct{} {
	normalized := make(map[string]struct{})

	for _, state := range states {
		trimmed := strings.ToLower(strings.TrimSpace(state))
		if trimmed == "" {
			continue
		}

		normalized[trimmed] = struct{}{}
	}

	return normalized
}
