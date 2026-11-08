package httpkit

import (
	"context"
	"errors"
	"net"
)

// Reason is why a request did not reach the host.
type Reason string

const (
	// ReasonDNS is a name that did not resolve.
	ReasonDNS Reason = "dns"
	// ReasonTimeout is a deadline, a connect timeout, or a net.Error that timed out.
	ReasonTimeout Reason = "timeout"
	// ReasonRefused is a connect the host rejected, including Windows WSAECONNREFUSED.
	ReasonRefused Reason = "refused"
	// ReasonOther is any other failure, including a nil error.
	ReasonOther Reason = "other"
)

// NetReason reports why a request did not reach the host: a name that did not resolve, a connect the
// host refused (including Windows WSAECONNREFUSED), a timeout (a deadline, ETIMEDOUT / WSAETIMEDOUT,
// or a net.Error that timed out), or ReasonOther.
func NetReason(err error) Reason {
	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return ReasonDNS
	}
	if errors.Is(err, refusedErrno) {
		return ReasonRefused
	}
	ne, ok := errors.AsType[net.Error](err)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, timedOutErrno) || (ok && ne.Timeout()) {
		return ReasonTimeout
	}
	return ReasonOther
}
