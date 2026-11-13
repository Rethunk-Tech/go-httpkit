package httpkit

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
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
// host other than the first request's, or from https to plain http, which would send them in clear text. Once any hop
// has left that origin they stay stripped for the rest of the chain, as net/http does. It never refuses a redirect.
func StripCrossHost(names ...string) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 {
			return nil
		}
		first := via[0].URL
		left := func(r *http.Request) bool {
			return hostKey(r.URL) != hostKey(first) || first.Scheme == "https" && r.URL.Scheme != "https"
		}
		// net/http copies the first request's headers onto every hop, so a chain back to the origin would restore them.
		if !left(req) && !slices.ContainsFunc(via[1:], left) {
			return nil
		}
		// delete, not Header.Del: a key set straight into the map need not be canonical, and Del canonicalizes.
		for k := range req.Header {
			lower := strings.ToLower(k)
			if slices.ContainsFunc(credentialWords, func(w string) bool { return strings.Contains(lower, w) }) ||
				slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, k) }) {
				delete(req.Header, k)
			}
		}
		return nil
	}
}

var defaultPort = map[string]string{"http": "80", "https": "443"}

// hostKey is u's host with ASCII letters lowered and the scheme's default port dropped, so two spellings of one
// host compare equal. Other letters are left as is: a stricter compare only strips or refuses more.
func hostKey(u *url.URL) string {
	port := u.Port()
	if port == defaultPort[u.Scheme] {
		port = ""
	}
	lower := func(r rune) rune {
		if 'A' <= r && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}
	return net.JoinHostPort(strings.Map(lower, u.Hostname()), port)
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
	if first := via[0].URL; hostKey(req.URL) != hostKey(first) || req.URL.Scheme != first.Scheme {
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
