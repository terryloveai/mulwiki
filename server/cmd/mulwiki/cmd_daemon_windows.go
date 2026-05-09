//go:build windows

package main

import (
	"errors"
	"syscall"
)

const (
	detachedProcess        = 0x00000008
	createBreakawayFromJob = 0x01000000
)

// daemonSysProcAttr returns attributes used when spawning the background daemon.
// On Windows, detachedProcess severs console inheritance and breakaway lets the
// child survive parent Job Object close when the parent permits it.
func daemonSysProcAttr(withBreakaway bool) *syscall.SysProcAttr {
	flags := uint32(detachedProcess)
	if withBreakaway {
		flags |= createBreakawayFromJob
	}
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: flags,
	}
}

func isAccessDeniedSpawnErr(err error) bool {
	return errors.Is(err, syscall.ERROR_ACCESS_DENIED)
}
