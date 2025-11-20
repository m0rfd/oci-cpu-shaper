package imds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	errEmptyResponse        = errors.New("imds: empty response")
	errEmptyCanonicalRegion = errors.New("imds: empty canonicalRegionName response")
)

// Region returns the canonical region for the running instance.
func (c *HTTPClient) Region(ctx context.Context) (string, error) {
	body, err := c.getText(ctx, "region")
	if err != nil {
		return "", err
	}

	return body, nil
}

// CanonicalRegion returns the canonical region name for the running instance.
func (c *HTTPClient) CanonicalRegion(ctx context.Context) (string, error) {
	var metadata instanceMetadata

	err := c.getJSON(ctx, "", &metadata)
	if err != nil {
		return "", err
	}

	canonical := strings.TrimSpace(metadata.RegionInfo.CanonicalRegionName)
	if canonical == "" {
		return "", errEmptyCanonicalRegion
	}

	return canonical, nil
}

// InstanceID returns the OCID for the running instance.
func (c *HTTPClient) InstanceID(ctx context.Context) (string, error) {
	body, err := c.getText(ctx, "id")
	if err != nil {
		return "", err
	}

	return body, nil
}

// CompartmentID returns the compartment OCID for the running instance.
func (c *HTTPClient) CompartmentID(ctx context.Context) (string, error) {
	body, err := c.getText(ctx, "compartmentId")
	if err != nil {
		return "", err
	}

	return body, nil
}

// ShapeConfig returns the compute shape metadata for the running instance.
func (c *HTTPClient) ShapeConfig(ctx context.Context) (ShapeConfig, error) {
	var cfg ShapeConfig

	err := c.getJSON(ctx, "shapeConfig", &cfg)
	if err != nil {
		return ShapeConfig{}, err
	}

	return cfg, nil
}

func (c *HTTPClient) getText(ctx context.Context, resource string) (string, error) {
	payload, err := c.fetch(ctx, resource)
	if err != nil {
		return "", err
	}

	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" {
		return "", fmt.Errorf("%s: %w", resource, errEmptyResponse)
	}

	return trimmed, nil
}

func (c *HTTPClient) getJSON(ctx context.Context, resource string, out any) error {
	payload, err := c.fetch(ctx, resource)
	if err != nil {
		return err
	}

	decodeErr := json.Unmarshal(payload, out)
	if decodeErr != nil {
		return fmt.Errorf("decode %s response: %w", resource, decodeErr)
	}

	return nil
}

type regionInfo struct {
	CanonicalRegionName string `json:"canonicalRegionName"`
}

type instanceMetadata struct {
	RegionInfo regionInfo `json:"regionInfo"`
}
