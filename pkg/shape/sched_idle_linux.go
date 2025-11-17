//go:build linux && rootful

package shape

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

var (
	schedSetAttrMu sync.RWMutex
	schedSetAttr   = unix.SchedSetAttr
)

func trySchedIdle() error {
	schedSetAttrMu.RLock()
	fn := schedSetAttr
	schedSetAttrMu.RUnlock()

	attr := &unix.SchedAttr{
		Size:   uint32(unsafe.Sizeof(unix.SchedAttr{})),
		Policy: unix.SCHED_IDLE,
	}

	return fn(0, attr, 0)
}
