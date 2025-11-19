package main

import (
	"context"
	"errors"
	"testing"

	runtimeconfig "oci-cpu-shaper/pkg/runtimeconfig"
)

func TestResolveInstanceIDUsesConfiguredValue(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.InstanceID = "  ocid1.instance.oc1..configured  "

	imdsClient := &stubIMDSClient{ //nolint:exhaustruct
		instanceID: "  ocid1.instance.oc1..imds  ",
	}

	got, err := resolveInstanceID(context.Background(), cfg, false, imdsClient)
	if err != nil {
		t.Fatalf("resolveInstanceID returned error: %v", err)
	}

	const wantInstanceID = "ocid1.instance.oc1..configured"

	if got != wantInstanceID {
		t.Fatalf("expected instance ID %q, got %q", wantInstanceID, got)
	}

	if imdsClient.instanceCalls != 0 {
		t.Fatalf(
			"expected no IMDS calls when instance ID override is provided, got %d",
			imdsClient.instanceCalls,
		)
	}
}

func TestResolveInstanceIDOfflineFallback(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()
	cfg.OCI.Offline = true

	imdsClient := &stubIMDSClient{ //nolint:exhaustruct
		instanceID: "ocid1.instance.oc1..ignored",
	}

	got, err := resolveInstanceID(context.Background(), cfg, true, imdsClient)
	if err != nil {
		t.Fatalf("resolveInstanceID returned error: %v", err)
	}

	if got != offlineInstanceFallback {
		t.Fatalf("expected offline fallback %q, got %q", offlineInstanceFallback, got)
	}

	if imdsClient.instanceCalls != 0 {
		t.Fatalf("expected no IMDS calls in offline mode, got %d", imdsClient.instanceCalls)
	}
}

func TestResolveInstanceIDUsesIMDSWhenOverrideMissing(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()

	imdsClient := &stubIMDSClient{ //nolint:exhaustruct
		instanceID: "   ocid1.instance.oc1..imds   ",
	}

	got, err := resolveInstanceID(context.Background(), cfg, false, imdsClient)
	if err != nil {
		t.Fatalf("resolveInstanceID returned error: %v", err)
	}

	const wantInstanceID = "ocid1.instance.oc1..imds"

	if got != wantInstanceID {
		t.Fatalf("expected instance ID %q, got %q", wantInstanceID, got)
	}

	if imdsClient.instanceCalls != 1 {
		t.Fatalf("expected one IMDS call, got %d", imdsClient.instanceCalls)
	}
}

func TestResolveInstanceIDPropagatesIMDSErrors(t *testing.T) {
	t.Parallel()

	cfg := runtimeconfig.Default()

	imdsClient := &stubIMDSClient{ //nolint:exhaustruct
		instanceErr: errInstanceDown,
	}

	_, err := resolveInstanceID(context.Background(), cfg, false, imdsClient)
	if err == nil || !errors.Is(err, errInstanceDown) {
		t.Fatalf("expected errInstanceDown, got %v", err)
	}
}
