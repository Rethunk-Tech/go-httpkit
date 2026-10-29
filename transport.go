package httpkit

import "net/http"

// Transport sets a default User-Agent and reports every round trip.
type Transport struct {
	Base      http.RoundTripper // Nil means http.DefaultTransport.
	UserAgent string            // Set only when the request has none.
	// OnResult sees every round trip's response or transport error; nil means none. It must not read resp.Body.
	OnResult func(req *http.Request, resp *http.Response, err error)
}

// RoundTrip never modifies req: when it sets the User-Agent it does so on req.Clone(req.Context()).
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if t.UserAgent != "" && req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", t.UserAgent)
	}
	resp, err := base.RoundTrip(req)
	if t.OnResult != nil {
		t.OnResult(req, resp, err)
	}
	return resp, err
}

// CloseIdleConnections forwards to Base when Base has the method, so http.Client.CloseIdleConnections still works.
func (t *Transport) CloseIdleConnections() {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if c, ok := base.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}
