package oci //nolint:testpackage

import (
	"context"
	"testing"
)

func TestNewStaticMetricsClientReturnsConstantValue(t *testing.T) {
	t.Parallel()

	client := NewStaticMetricsClient(0.37)

	value, err := client.QueryP95CPU(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("QueryP95CPU returned error: %v", err)
	}

	if value != 0.37 {
		t.Fatalf("expected static value 0.37, got %v", value)
	}
}
