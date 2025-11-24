package main

import (
	"errors"
	"strings"
	"testing"

	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

var (
	errMissingOverride  = errors.New("missing")
	errFetchBoom        = errors.New("boom")
	errLegacyBoom       = errors.New("legacy boom")
	errCanonicalBoom    = errors.New("canonical boom")
	errLegacyMissing    = errors.New("legacy missing")
	errCanonicalMissing = errors.New("canonical missing")
)

func TestPreferMetadataValuePrioritisesOverrideAndFetched(t *testing.T) {
	t.Parallel()

	t.Run("override wins", func(t *testing.T) {
		t.Parallel()

		value, err := preferMetadataValue(
			"fetched",
			nil,
			"  override  ",
			errMissingOverride,
			"prefix",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if value != "override" {
			t.Fatalf("expected override to win, got %q", value)
		}
	})

	t.Run("fetched used when override empty", func(t *testing.T) {
		t.Parallel()

		value, err := preferMetadataValue(" fetched\n", nil, "", errMissingOverride, "prefix")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if value != "fetched" {
			t.Fatalf("expected trimmed fetched value, got %q", value)
		}
	})
}

func TestPreferMetadataValueReportsErrors(t *testing.T) {
	t.Parallel()

	t.Run("returns fetch error with prefix", func(t *testing.T) {
		t.Parallel()

		_, err := preferMetadataValue("", errFetchBoom, "", errMissingOverride, "lookup metadata")
		if err == nil || !strings.Contains(err.Error(), "lookup metadata") {
			t.Fatalf("expected wrapped fetch error, got %v", err)
		}
	})

	t.Run("returns missing error without fetch error", func(t *testing.T) {
		t.Parallel()

		_, err := preferMetadataValue("", nil, "\t", errMissingOverride, "lookup metadata")
		if !errors.Is(err, errMissingOverride) {
			t.Fatalf("expected missing error, got %v", err)
		}
	})
}

func TestPrepareRunMetadataMergesResolvedValues(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = " \t"
	cfg.OCI.Region = ""

	client := &stubIMDSClient{ //nolint:exhaustruct
		compartmentID: testCompartmentOverride,
		region:        stubRegion,
	}

	updated, metadata, err := prepareRunMetadata(t.Context(), cfg, client, modeDryRun)
	if err != nil {
		t.Fatalf("prepareRunMetadata returned error: %v", err)
	}

	if metadata.CompartmentID != testCompartmentOverride {
		t.Fatalf("expected compartment override to be resolved, got %q", metadata.CompartmentID)
	}

	if metadata.Region != stubRegion {
		t.Fatalf("expected region to be resolved, got %q", metadata.Region)
	}

	if updated.OCI.CompartmentID != testCompartmentOverride {
		t.Fatalf("expected config compartment to be updated, got %q", updated.OCI.CompartmentID)
	}

	if updated.OCI.Region != stubRegion {
		t.Fatalf("expected config region to be updated, got %q", updated.OCI.Region)
	}
}

func TestPrepareRunMetadataSkipsNoopMode(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	client := new(stubIMDSClient)

	updated, metadata, err := prepareRunMetadata(t.Context(), cfg, client, modeNoop)
	if err != nil {
		t.Fatalf("prepareRunMetadata returned error: %v", err)
	}

	var expected ociMetadata
	if metadata != expected {
		t.Fatalf("expected empty metadata, got %+v", metadata)
	}

	if updated.OCI != cfg.OCI {
		t.Fatalf("expected OCI config to remain unchanged, got %+v", updated.OCI)
	}
}

func TestPrepareRunMetadataPropagatesResolutionErrors(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.CompartmentID = testCompartmentOverride
	client := new(stubIMDSClient)

	_, _, err := prepareRunMetadata(t.Context(), cfg, client, modeEnforce)
	if err == nil || !errors.Is(err, errControllerRegionRequired) {
		t.Fatalf("expected errControllerRegionRequired, got %v", err)
	}
}

func TestPreferCanonicalRegionValueOrder(t *testing.T) {
	t.Parallel()

	override := "  phx-override  "

	value, err := preferCanonicalRegionValue("ignored", nil, "ignored", nil, override)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value != "phx-override" {
		t.Fatalf("expected override to be trimmed, got %q", value)
	}

	value, err = preferCanonicalRegionValue("phx-a", nil, "phx-b", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value != "phx-a" {
		t.Fatalf("expected canonical region to win, got %q", value)
	}

	value, err = preferCanonicalRegionValue(
		"",
		errCanonicalBoom,
		" phx-legacy ",
		errLegacyBoom,
		"",
	)
	if err != nil {
		t.Fatalf("expected legacy region despite error, got %v", err)
	}

	if value != "phx-legacy" {
		t.Fatalf("expected trimmed legacy region, got %q", value)
	}
}

func TestPreferCanonicalRegionValueErrorPropagation(t *testing.T) {
	t.Parallel()

	_, err := preferCanonicalRegionValue("", nil, "", errLegacyMissing, "")
	if err == nil || !strings.Contains(err.Error(), "lookup instance region") {
		t.Fatalf("expected legacy lookup error, got %v", err)
	}

	_, err = preferCanonicalRegionValue("", errCanonicalMissing, "", nil, "")
	if err == nil || !errors.Is(err, errCanonicalMissing) {
		t.Fatalf("expected canonical lookup error, got %v", err)
	}

	_, err = preferCanonicalRegionValue("", nil, "", nil, "")
	if !errors.Is(err, errControllerRegionRequired) {
		t.Fatalf("expected region required error, got %v", err)
	}
}
