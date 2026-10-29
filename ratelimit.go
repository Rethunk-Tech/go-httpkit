package httpkit

import (
	"net/http"
	"strconv"
	"time"
)

// RateLimitError is a server refusing calls until Reset (zero when it did not say).
type RateLimitError struct{ Reset time.Time }

func (e *RateLimitError) Error() string {
	if e.Reset.IsZero() {
		return "rate limited"
	}
	return "rate limited until " + e.Reset.Format(time.RFC3339)
}

// Is matches ErrRateLimited.
func (e *RateLimitError) Is(target error) bool { return target == ErrRateLimited }

// maxRetryAfter bounds a Retry-After in seconds so the Duration arithmetic cannot overflow.
const maxRetryAfter = 365 * 24 * 3600

// RetryAfter reads a Retry-After value (RFC 9110 section 10.2.3): delta-seconds from now, or an HTTP-date. ok is
// false when v is empty, unparseable, "0" or negative, or more than 365 days of seconds (so the Duration cannot
// overflow). A past HTTP-date is returned as is; the caller decides what a reset not after now means.
func RetryAfter(v string) (at time.Time, ok bool) {
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		if n <= 0 || n > maxRetryAfter {
			return time.Time{}, false
		}
		return time.Now().Add(time.Duration(n) * time.Second), true
	}
	t, err := http.ParseTime(v)
	return t, err == nil
}
