//go:build linux && rootful

package shape

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

type schedParam struct {
	priority int32
	_        int32 // align to 8 bytes on 64-bit kernels
}

type schedIdleSetter interface {
	setScheduler(pid int, policy int, param *schedParam) error
}

type unixSchedIdleSetter struct{}

func (unixSchedIdleSetter) setScheduler(pid int, policy int, param *schedParam) error {
	if param == nil {
		param = &schedParam{}
	}

	_, _, errno := unix.RawSyscall(
		unix.SYS_SCHED_SETSCHEDULER,
		uintptr(pid),
		uintptr(policy),
		uintptr(unsafe.Pointer(param)),
	)
	if errno != 0 {
		return errno
	}

	return nil
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

	if err := setter.setScheduler(0, unix.SCHED_IDLE, &schedParam{}); err != nil {
		return err
	}

	hasSysNice, err := hasSysNiceCapability()
	if err != nil {
		return err
	}

	if !hasSysNice {
		return unix.EPERM
	}

	return nil
}

func hasSysNiceCapability() (bool, error) {
	header := unix.CapUserHeader{
		Version: unix.LINUX_CAPABILITY_VERSION_3,
		Pid:     0,
	}
	data := [2]unix.CapUserData{}

	if err := unix.Capget(&header, &data[0]); err != nil {
		return false, err
	}

	const sysNiceMask = 1 << uint(unix.CAP_SYS_NICE)

	return data[0].Effective&sysNiceMask != 0, nil
}
