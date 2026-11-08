//go:build !windows

package httpkit_test

import "syscall"

const (
	connectCall = "connect"
	osRefused   = syscall.ECONNREFUSED
	osTimedOut  = syscall.ETIMEDOUT
)
