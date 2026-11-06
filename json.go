package httpkit

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// DefaultJSONLimit is the body cap DoJSON applies when JSONOptions.Limit is zero.
const DefaultJSONLimit = 16 << 20

// JSONOptions tunes DoJSON. The zero value is valid.
type JSONOptions struct {
	// Limit caps the body read, in bytes; zero means DefaultJSONLimit.
	Limit int64
	// Check runs on every response before the status test, ahead of the 429 test, so a caller can claim any
	// status itself (a 403 that means "rate limited" for one API and "not allowed" for another). A non-nil error
	// is returned as is.
	Check func(*http.Response) error
}

// DoJSON sends req with hc (nil means http.DefaultClient) and decodes a 200 answer's JSON body into out. A 429 is a
// *RateLimitError carrying the Retry-After reset, any other status a *StatusError, a body over the limit
// safefs.ErrTooLarge, and malformed JSON an error wrapping the decoder's. Accept is set to application/json when
// req names none.
func DoJSON(hc *http.Client, req *http.Request, out any, o JSONOptions) error {
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	limit := o.Limit
	if limit <= 0 {
		limit = DefaultJSONLimit
	}
	raw, err := DoCapped(hc, req, limit, func(resp *http.Response) error {
		if o.Check != nil {
			if err := o.Check(resp); err != nil {
				return err
			}
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			return nil
		}
		e := &RateLimitError{}
		if t, ok := RetryAfter(resp.Header.Get("Retry-After")); ok {
			e.Reset = t
		}
		return e
	})
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
