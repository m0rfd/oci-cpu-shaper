package shape

import (
	"runtime"
	"time"
)

func busyWait(duration time.Duration) {
	if duration <= 0 {
		return
	}

	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		runtime.Gosched()
	}
}
