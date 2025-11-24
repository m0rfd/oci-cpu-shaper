package main

import (
	"context"
	"fmt"
	"strings"

	"oci-cpu-shaper/pkg/imds"
	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

func resolveInstanceID(
	ctx context.Context,
	cfg runtimeconfig.Config,
	offline bool,
	imdsClient imds.Client,
) (string, error) {
	instanceID := strings.TrimSpace(cfg.OCI.InstanceID)
	if instanceID != "" {
		return instanceID, nil
	}

	if offline {
		return offlineInstanceFallback, nil
	}

	fetchedID, err := imdsClient.InstanceID(ctx)
	if err != nil {
		return "", fmt.Errorf("lookup instance ocid: %w", err)
	}

	return strings.TrimSpace(fetchedID), nil
}

type ociMetadata struct {
	CompartmentID string
	Region        string
}

func resolveCompartmentAndRegion(
	ctx context.Context,
	cfg runtimeconfig.Config,
	imdsClient imds.Client,
) (ociMetadata, error) {
	compartmentOverride := strings.TrimSpace(cfg.OCI.CompartmentID)
	regionOverride := strings.TrimSpace(cfg.OCI.Region)

	metadata := ociMetadata{
		CompartmentID: compartmentOverride,
		Region:        regionOverride,
	}

	if cfg.OCI.Offline {
		return metadata, nil
	}

	if imdsClient == nil {
		return ociMetadata{}, errControllerIMDSRequired
	}

	if metadata.CompartmentID != "" && metadata.Region != "" {
		return metadata, nil
	}

	if metadata.CompartmentID == "" {
		compartmentID, compartmentErr := imdsClient.CompartmentID(ctx)

		value, err := preferMetadataValue(
			compartmentID,
			compartmentErr,
			compartmentOverride,
			errControllerCompartmentRequired,
			"lookup compartment ocid",
		)
		if err != nil {
			return ociMetadata{}, err
		}

		metadata.CompartmentID = value
	}

	if metadata.Region == "" {
		canonicalRegion, canonicalRegionErr := imdsClient.CanonicalRegion(ctx)
		region, regionErr := imdsClient.Region(ctx)

		value, err := preferCanonicalRegionValue(
			canonicalRegion,
			canonicalRegionErr,
			region,
			regionErr,
			regionOverride,
		)
		if err != nil {
			return ociMetadata{}, err
		}

		metadata.Region = value
	}

	return metadata, nil
}

func preferMetadataValue(
	fetched string,
	fetchErr error,
	override string,
	missingErr error,
	errPrefix string,
) (string, error) {
	trimmedOverride := strings.TrimSpace(override)
	if trimmedOverride != "" {
		return trimmedOverride, nil
	}

	trimmedFetched := strings.TrimSpace(fetched)
	if trimmedFetched != "" {
		return trimmedFetched, nil
	}

	if fetchErr != nil {
		return "", fmt.Errorf("%s: %w", errPrefix, fetchErr)
	}

	return "", missingErr
}

func preferCanonicalRegionValue(
	canonical string,
	canonicalErr error,
	legacy string,
	legacyErr error,
	override string,
) (string, error) {
	trimmedOverride := strings.TrimSpace(override)
	if trimmedOverride != "" {
		return trimmedOverride, nil
	}

	trimmedCanonical := strings.TrimSpace(canonical)
	if trimmedCanonical != "" && canonicalErr == nil {
		return trimmedCanonical, nil
	}

	trimmedLegacy := strings.TrimSpace(legacy)
	if trimmedLegacy != "" && legacyErr == nil {
		return trimmedLegacy, nil
	}

	if trimmedLegacy != "" {
		return trimmedLegacy, nil
	}

	if legacyErr != nil {
		return "", fmt.Errorf("lookup instance region: %w", legacyErr)
	}

	if canonicalErr != nil {
		return "", fmt.Errorf("lookup canonical region: %w", canonicalErr)
	}

	return "", errControllerRegionRequired
}

func prepareRunMetadata(
	ctx context.Context,
	cfg runtimeconfig.Config,
	imdsClient imds.Client,
	mode string,
) (runtimeconfig.Config, ociMetadata, error) {
	trimmedMode := strings.TrimSpace(mode)
	if trimmedMode == modeNoop {
		var empty ociMetadata

		return cfg, empty, nil
	}

	metadata, err := resolveCompartmentAndRegion(ctx, cfg, imdsClient)
	if err != nil {
		return cfg, ociMetadata{}, err
	}

	if metadata.CompartmentID != "" {
		cfg.OCI.CompartmentID = metadata.CompartmentID
	}

	if metadata.Region != "" {
		cfg.OCI.Region = metadata.Region
	}

	return cfg, metadata, nil
}
