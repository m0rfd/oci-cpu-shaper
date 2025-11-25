//go:build integration

package integration

import (
	"context"
	"runtime"
	"testing"

	"oci-cpu-shaper/pkg/est"
)

func TestFileSourceSnapshotDefaultPath(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "linux" {
		t.Skip("Test requires Linux /proc/stat")
	}

	snap, err := (est.FileSource{Path: ""}).Snapshot(context.Background())
	if err != nil {
		t.Fatalf("expected default /proc/stat to be readable, got %v", err)
	}

	if snap.Total == 0 {
		t.Fatalf("expected non-zero total jiffies from default path")
	}
}
