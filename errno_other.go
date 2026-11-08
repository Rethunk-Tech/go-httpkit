//go:build !windows

package httpkit

import "syscall"

const (
	refusedErrno  = syscall.ECONNREFUSED
	timedOutErrno = syscall.ETIMEDOUT
)
