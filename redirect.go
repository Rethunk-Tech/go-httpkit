package httpkit

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultTimeout bounds a whole request, body included, for a nil client: http.DefaultClient has none, so a server
// that stalls a body would hold the caller forever.
const DefaultTimeout = 5 * time.Minute

// defaultClient is what a nil *http.Client means in this module.
func defaultClient() *http.Client { return &http.Client{Timeout: DefaultTimeout} }

// credentialWords mark a header name that carries a secret. net/http strips only Authorization, Cookie and
// WWW-Authenticate off a cross-host redirect, so an API key header would follow it.
var credentialWords = []string{"auth", "key", "token", "secret", "cookie", "session", "credential", "password"}

// StripCrossHost returns a redirect policy that deletes the named headers, and every header whose name looks like a
// credential (it holds auth, key, token, secret, cookie, session, credential or password), from a request redirected to a
// host other than the first request's. It never refuses a redirect.
func StripCrossHost(names ...string) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 || req.URL.Host == via[0].URL.Host {
			return nil
		}
		for k := range req.Header {
			lower := strings.ToLower(k)
			for _, w := range credentialWords {
				if strings.Contains(lower, w) {
					req.Header.Del(k)
					break
				}
			}
		}
		for _, n := range names {
			req.Header.Del(n)
		}
		return nil
	}
}

// RedactURL is raw without its query, fragment and userinfo: a query often carries a signature or an API key, and
// none of it belongs in an error text or a log.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(unparseable URL)"
	}
	u.RawQuery, u.Fragment, u.User = "", "", nil
	return u.String()
}

// RedactError returns err with the URL inside a *url.Error (what net/http wraps a failed request in) redacted by
// RedactURL.
func RedactError(err error) error {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		return &url.Error{Op: ue.Op, URL: RedactURL(ue.URL), Err: ue.Err}
	}
	return err
}

// maxRedirects is net/http's own default hop limit.
const maxRedirects = 10

var errTooManyRedirects = errors.New("stopped after 10 redirects")

// NoDowngrade refuses a redirect from https to any other scheme: net/http keeps the Authorization header across a
// same-host redirect, scheme change included. A first hop that is not https may redirect freely. A redirect to
// another host loses every credential-looking header (see StripCrossHost).
func NoDowngrade(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errTooManyRedirects
	}
	_ = StripCrossHost()(req, via)
	if req.URL.Scheme != "https" && via[0].URL.Scheme == "https" {
		return fmt.Errorf("refusing a redirect to %s://", req.URL.Scheme)
	}
	return nil
}

// SameOrigin refuses a redirect to another scheme or host than the first request's, so a header such as an API key
// stays with its origin.
func SameOrigin(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errTooManyRedirects
	}
	if first := via[0].URL; req.URL.Host != first.Host || req.URL.Scheme != first.Scheme {
		return fmt.Errorf("refusing a redirect to %s://%s", req.URL.Scheme, req.URL.Host)
	}
	return nil
}

// NoFollow returns the redirect answer itself instead of following it.
func NoFollow(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// WithRedirect returns a shallow copy of hc (nil means a client with DefaultTimeout) whose CheckRedirect runs hc's own
// policy first, when it has one, and then policy. hc is never modified. A nil hc gets a client with DefaultTimeout.
func WithRedirect(hc *http.Client, policy func(*http.Request, []*http.Request) error) *http.Client {
	if hc == nil {
		hc = defaultClient()
	}
	c := *hc
	inner := hc.CheckRedirect
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if inner != nil {
			if err := inner(req, via); err != nil {
				return err
			}
		}
		return policy(req, via)
	}
	return &c
}
