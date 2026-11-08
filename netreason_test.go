package httpkit_test

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"testing"

	"github.com/Rethunk-Tech/go-httpkit"
)

func TestNetReason(t *testing.T) {
	t.Parallel()
	refused := &net.OpError{Op: "proxyconnect", Net: "tcp", Err: &os.SyscallError{Syscall: connectCall, Err: osRefused}}
	timedOut := &net.OpError{Op: "dial", Net: "tcp", Err: &os.SyscallError{Syscall: connectCall, Err: osTimedOut}}
	for err, want := range map[error]httpkit.Reason{
		&url.Error{Op: "Get", URL: "https://example.com", Err: refused}:                            httpkit.ReasonRefused,
		&url.Error{Op: "Get", URL: "https://example.com", Err: timedOut}:                           httpkit.ReasonTimeout,
		&url.Error{Op: "Get", URL: "https://example.com", Err: &net.DNSError{Err: "no such host"}}: httpkit.ReasonDNS,
		context.DeadlineExceeded: httpkit.ReasonTimeout,
		&net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset by peer")}: httpkit.ReasonOther,
	} {
		if got := httpkit.NetReason(err); got != want {
			t.Fatalf("%v: reason %q, want %q", err, got, want)
		}
	}
}
