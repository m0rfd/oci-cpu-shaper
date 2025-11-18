//nolint:testpackage // busyWait is intentionally unexported
package shape

import (
	"testing"
	"time"
)

func TestBusyWaitHandlesDurations(t *testing.T) {
	t.Parallel()

	start := time.Now()

	busyWait(0)

	if elapsed := time.Since(start); elapsed > time.Millisecond {
		t.Fatalf("busyWait should return immediately for zero duration, took %v", elapsed)
	}

	start = time.Now()

	busyWait(200 * time.Microsecond)

	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("busyWait exceeded expected duration, took %v", elapsed)
	}
}
