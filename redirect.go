package httpkit

import (
	"errors"
	"fmt"
	"net/http"
)

// maxRedirects is net/http's own default hop limit.
const maxRedirects = 10

var errTooManyRedirects = errors.New("stopped after 10 redirects")

// NoDowngrade refuses a redirect from https to any other scheme: net/http keeps the Authorization header across a
// same-host redirect, scheme change included. A first hop that is not https may redirect freely.
func NoDowngrade(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errTooManyRedirects
	}
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

// WithRedirect returns a shallow copy of hc (nil means http.DefaultClient) whose CheckRedirect runs hc's own
// policy first, when it has one, and then policy. hc is never modified.
func WithRedirect(hc *http.Client, policy func(*http.Request, []*http.Request) error) *http.Client {
	if hc == nil {
		hc = http.DefaultClient
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
