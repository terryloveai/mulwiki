//go:build !windows

package main

import "syscall"

// daemonSysProcAttr returns attributes used when spawning the background daemon.
// Setsid detaches the child from the parent session and process group so it can
// survive the parent shell exiting.
func daemonSysProcAttr(_ bool) *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

func isAccessDeniedSpawnErr(_ error) bool {
	return false
}
