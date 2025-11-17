//go:build rootful

package shape

import (
	"errors"
	"sync"

	"golang.org/x/sys/unix"
)

type schedIdleSetter interface {
	setScheduler(pid int, policy int, param *unix.SchedParam) error
}

type unixSchedIdleSetter struct{}

func (unixSchedIdleSetter) setScheduler(pid int, policy int, param *unix.SchedParam) error {
	return unix.SchedSetScheduler(pid, policy, param)
}

var (
	schedIdleSetterMu      sync.RWMutex
	currentSchedIdleSetter schedIdleSetter = unixSchedIdleSetter{}
)

func trySchedIdle() error {
	schedIdleSetterMu.RLock()
	setter := currentSchedIdleSetter
	schedIdleSetterMu.RUnlock()

	if setter == nil {
		return nil
	}

	err := setter.setScheduler(0, unix.SCHED_IDLE, &unix.SchedParam{})
	if err == nil || errors.Is(err, unix.EPERM) {
		return nil
	}

	return err
}
