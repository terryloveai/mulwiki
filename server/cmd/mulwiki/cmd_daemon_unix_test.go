//go:build !windows

package main

import "testing"

func TestDaemonSysProcAttrDetachesUnixChild(t *testing.T) {
	attr := daemonSysProcAttr(true)
	if attr == nil {
		t.Fatal("daemonSysProcAttr returned nil")
	}
	if !attr.Setsid {
		t.Fatal("daemonSysProcAttr must set Setsid so background daemon survives parent shell exit")
	}
	if isAccessDeniedSpawnErr(assertionError{}) {
		t.Fatal("Unix access denied retry should always be disabled")
	}
}

type assertionError struct{}

func (assertionError) Error() string { return "assertion" }
