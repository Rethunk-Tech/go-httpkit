package httpkit

import "syscall"

// Winsock reports a refused or timed-out connect with its own codes, which syscall.ECONNREFUSED and Errno.Timeout do
// not match on Windows.
const (
	refusedErrno  = syscall.Errno(10061) // WSAECONNREFUSED
	timedOutErrno = syscall.Errno(10060) // WSAETIMEDOUT
)
