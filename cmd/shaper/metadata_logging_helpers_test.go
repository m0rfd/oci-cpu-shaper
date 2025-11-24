package main

import (
	"testing"

	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"oci-cpu-shaper/pkg/adapt"
)

func assertNoIMDSCalls(t *testing.T, client *stubIMDSClient) {
	t.Helper()

	if client.regionCalls != 0 || client.canonicalRegionCalls != 0 || client.instanceCalls != 0 ||
		client.compartmentCalls != 0 || client.shapeCalls != 0 {
		t.Fatalf(
			"expected offline mode to skip imds lookups, got region=%d canonical=%d instance=%d compartment=%d shape=%d",
			client.regionCalls,
			client.canonicalRegionCalls,
			client.instanceCalls,
			client.compartmentCalls,
			client.shapeCalls,
		)
	}
}

func requireOverrideIMDSLookups(t *testing.T, client *stubIMDSClient) {
	t.Helper()

	if client.regionCalls != 0 {
		t.Fatalf(
			"expected override to skip IMDS region lookup, got %d calls",
			client.regionCalls,
		)
	}

	if client.instanceCalls != 0 {
		t.Fatalf(
			"expected override to skip IMDS instance lookup, got %d calls",
			client.instanceCalls,
		)
	}

	if client.compartmentCalls != 0 {
		t.Fatalf("expected override to skip compartment lookup, got %d", client.compartmentCalls)
	}

	if client.canonicalRegionCalls != 1 {
		t.Fatalf(
			"expected canonical region lookup despite overrides, got %d",
			client.canonicalRegionCalls,
		)
	}
}

func assertOfflineLog(t *testing.T, observed *observer.ObservedLogs, expectedID string) {
	t.Helper()

	if warns := observed.FilterLevelExact(zapcore.WarnLevel).All(); len(warns) != 0 {
		t.Fatalf("expected no warnings, got %d", len(warns))
	}

	entries := observed.FilterLevelExact(zapcore.InfoLevel).All()
	if len(entries) != 1 {
		t.Fatalf("expected single info entry, got %d", len(entries))
	}

	entry := entries[0]
	if got := fieldString(entry.Context, "instanceID"); got != expectedID {
		t.Fatalf("expected trimmed override instance id, got %q", got)
	}

	if got := fieldString(entry.Context, "controllerState"); got != adapt.StateNormal.String() {
		t.Fatalf("expected controller state %q, got %q", adapt.StateNormal.String(), got)
	}

	offline, ok := fieldBool(entry.Context, "offline")
	if !ok || !offline {
		t.Fatalf("expected offline field to be true, got %v (ok=%v)", offline, ok)
	}
}
