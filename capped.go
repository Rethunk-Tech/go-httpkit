package httpkit

import (
	"errors"
	"net/http"

	safefs "github.com/Rethunk-Tech/go-safefs"
)

// DoCapped sends req with hc (nil means http.DefaultClient) and returns the body of a 200 answer, at most limit
// bytes. check (nil means none) runs on every response before the status test; a non-nil error from it is returned
// after the body is closed. Any other status is a *StatusError; a body over limit is safefs.ErrTooLarge. The body
// is always closed.
func DoCapped(hc *http.Client, req *http.Request, limit int64, check func(*http.Response) error) ([]byte, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req) //nolint:gosec // G704: the caller built req and owns its destination
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if check != nil {
		if err := check(resp); err != nil {
			return nil, err
		}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{Code: resp.StatusCode, Status: resp.Status}
	}
	return safefs.ReadCapped(resp.Body, limit)
}

var (
	// ErrNotFound is matched by a 404 *StatusError.
	ErrNotFound = errors.New("not found")
	// ErrRateLimited is matched by a 429 *StatusError and by every *RateLimitError.
	ErrRateLimited = errors.New("rate limited")
)

// StatusError is an answer whose status the caller did not accept.
type StatusError struct {
	Code   int
	Status string // resp.Status, e.g. "502 Bad Gateway".
}

func (e *StatusError) Error() string { return "server answered " + e.Status }

// Is matches ErrNotFound for a 404 and ErrRateLimited for a 429.
func (e *StatusError) Is(target error) bool {
	switch target {
	case ErrNotFound:
		return e.Code == http.StatusNotFound
	case ErrRateLimited:
		return e.Code == http.StatusTooManyRequests
	}
	return false
}
