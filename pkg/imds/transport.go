package imds

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const metadataAuthorization = "Bearer Oracle"

var (
	errRetryableStatus  = errors.New("imds: retryable status code")
	errUnexpectedStatus = errors.New("imds: unexpected status code")
	errExhaustedRetries = errors.New("imds: exhausted retry budget")
	errRequestFailed    = errors.New("imds: request execution failed")
)

func (c *HTTPClient) fetch(ctx context.Context, resource string) ([]byte, error) {
	var lastErr error

	for attempt := 1; attempt <= c.maxAttempt; attempt++ {
		payload, retry, err := c.tryFetch(ctx, resource)
		if err == nil {
			return payload, nil
		}

		if !retry {
			return nil, err
		}

		lastErr = err

		if attempt == c.maxAttempt {
			break
		}

		waitErr := c.wait(ctx)
		if waitErr != nil {
			return nil, fmt.Errorf("retry wait for %s: %w", resource, waitErr)
		}
	}

	if lastErr == nil {
		return nil, fmt.Errorf("%w: %s", errExhaustedRetries, resource)
	}

	return nil, fmt.Errorf("%w: %w", errExhaustedRetries, lastErr)
}

func (c *HTTPClient) wait(ctx context.Context) error {
	timer := time.NewTimer(c.backoff)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("context done while waiting to retry: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

func (c *HTTPClient) tryFetch(ctx context.Context, resource string) ([]byte, bool, error) {
	req, err := metadataRequest(ctx, http.MethodGet, c.resourceURL(resource))
	if err != nil {
		return nil, false, fmt.Errorf("build request for %s: %w", resource, err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		ctxErr := ctx.Err()
		if ctxErr != nil {
			return nil, false, fmt.Errorf("%w: %s: %w", errRequestFailed, resource, ctxErr)
		}

		return nil, true, fmt.Errorf("%w: %s: %w", errRequestFailed, resource, err)
	}

	body, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()

	if readErr != nil {
		if closeErr != nil {
			wrap := fmt.Errorf("close response body: %w", closeErr)
			readErr = errors.Join(readErr, wrap)
		}

		return nil, false, fmt.Errorf("read %s response: %w", resource, readErr)
	}

	if closeErr != nil {
		return nil, false, fmt.Errorf("close %s response body: %w", resource, closeErr)
	}

	if resp.StatusCode == http.StatusOK {
		return body, false, nil
	}

	if !isRetryable(resp.StatusCode) {
		trimmed := strings.TrimSpace(string(body))

		return nil, false, fmt.Errorf(
			"%w: %s (status %d, body %s)",
			errUnexpectedStatus,
			resource,
			resp.StatusCode,
			trimmed,
		)
	}

	return nil, true, fmt.Errorf(
		"%w: %s (status %d)",
		errRetryableStatus,
		resource,
		resp.StatusCode,
	)
}

func (c *HTTPClient) resourceURL(resource string) string {
	trimmed := strings.TrimPrefix(resource, "/")
	base := strings.TrimRight(c.baseURL, "/")

	return fmt.Sprintf("%s/instance/%s", base, trimmed)
}

func isRetryable(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return true
	default:
		return status >= 500 && status != http.StatusNotImplemented
	}
}

func metadataRequest(ctx context.Context, method, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build metadata request: %w", err)
	}

	req.Header.Set("Authorization", metadataAuthorization)

	return req, nil
}
