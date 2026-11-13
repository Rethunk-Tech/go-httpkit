package httpkit_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Rethunk-Tech/go-httpkit"
)

func chain(t *testing.T, urls ...string) (*http.Request, []*http.Request) {
	t.Helper()
	var reqs []*http.Request
	for _, u := range urls {
		pu, err := url.Parse(u)
		if err != nil {
			t.Fatal(err)
		}
		reqs = append(reqs, &http.Request{URL: pu})
	}
	return reqs[len(reqs)-1], reqs[:len(reqs)-1]
}

func TestNoDowngradeAndSameOrigin(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name              string
		first, next       string
		noDowngrade, same bool // Whether the policy lets the redirect through.
	}{
		{"https to https elsewhere", "https://a.example/x", "https://b.example/y", true, false},
		{"https to http", "https://a.example/x", "http://a.example/y", false, false},
		{"same origin", "https://a.example/x", "https://a.example/z", true, true},
		{"another port", "https://a.example/x", "https://a.example:8443/z", true, false},
		{"http first hop is free", "http://a.example/x", "https://b.example/y", true, false},
		{"http to http", "http://a.example/x", "http://a.example/y", true, true},
		{"host case", "https://A.Example/x", "https://a.example/y", true, true},
		{"explicit default port", "https://a.example:443/x", "https://a.example/y", true, true},
		{"trailing dot", "https://a.example/x", "https://a.example./y", true, false},
	} {
		req, via := chain(t, c.first, c.next)
		if got := httpkit.NoDowngrade(req, via) == nil; got != c.noDowngrade {
			t.Errorf("NoDowngrade %s: allowed = %v", c.name, got)
		}
		if got := httpkit.SameOrigin(req, via) == nil; got != c.same {
			t.Errorf("SameOrigin %s: allowed = %v", c.name, got)
		}
	}
}

func TestPoliciesStopAfterTenHops(t *testing.T) {
	t.Parallel()
	urls := make([]string, 0, 11)
	for range 10 {
		urls = append(urls, "https://a.example/")
	}
	urls = append(urls, "https://a.example/next")
	req, via := chain(t, urls...)
	if httpkit.NoDowngrade(req, via) == nil || httpkit.SameOrigin(req, via) == nil {
		t.Fatal("the tenth redirect must stop")
	}
}

func TestNoFollowReturnsTheRedirect(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/elsewhere", http.StatusSeeOther)
	}))
	t.Cleanup(srv.Close)
	hc := httpkit.WithRedirect(srv.Client(), httpkit.NoFollow)
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/elsewhere" {
		t.Fatalf("got %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestWithRedirectChainsAndLeavesTheClientAlone(t *testing.T) {
	t.Parallel()
	errFirst, errSecond := errors.New("caller's policy"), errors.New("added policy")
	var order []string
	own := func(*http.Request, []*http.Request) error { order = append(order, "own"); return nil }
	hc := &http.Client{CheckRedirect: own}
	wrapped := httpkit.WithRedirect(hc, func(*http.Request, []*http.Request) error { order = append(order, "added"); return errSecond })
	req, via := chain(t, "https://a.example/", "https://a.example/b")
	if err := wrapped.CheckRedirect(req, via); !errors.Is(err, errSecond) || len(order) != 2 || order[0] != "own" {
		t.Fatalf("err = %v, order %v", err, order)
	}
	if hc.CheckRedirect == nil || wrapped == hc {
		t.Fatal("the original client was replaced")
	}

	failing := httpkit.WithRedirect(&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return errFirst }},
		func(*http.Request, []*http.Request) error {
			t.Fatal("ran after the caller's policy refused")
			return nil
		})
	if err := failing.CheckRedirect(req, via); !errors.Is(err, errFirst) {
		t.Fatalf("err = %v", err)
	}

	if httpkit.WithRedirect(nil, httpkit.NoFollow) == http.DefaultClient || http.DefaultClient.CheckRedirect != nil {
		t.Fatal("nil must copy http.DefaultClient, not change it")
	}
}

// seenHeaders runs a request through a redirect from one real server to another and returns the headers the second saw.
func seenHeaders(t *testing.T, hc *http.Client, set map[string]string) http.Header {
	t.Helper()
	var got http.Header
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer origin.Close()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, origin.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range set {
		req.Header.Set(k, v)
	}
	if _, err := httpkit.DoCapped(hc, req, 1<<10, nil); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestACredentialHeaderDoesNotFollowARedirectToAnotherHost(t *testing.T) {
	got := seenHeaders(t, httpkit.WithRedirect(nil, httpkit.NoDowngrade),
		map[string]string{"X-Api-Key": "k", "X-Session-Id": "s", "Accept": "text/plain"})
	if got.Get("X-Api-Key") != "" || got.Get("X-Session-Id") != "" {
		t.Fatalf("credentials crossed hosts: %v", got)
	}
	if got.Get("Accept") != "text/plain" {
		t.Fatalf("an ordinary header was dropped: %v", got)
	}
}

func TestStripCrossHostDropsTheNamedHeaders(t *testing.T) {
	got := seenHeaders(t, httpkit.WithRedirect(nil, httpkit.StripCrossHost("X-Customer")), map[string]string{"X-Customer": "c"})
	if got.Get("X-Customer") != "" {
		t.Fatalf("named header crossed hosts: %v", got)
	}
}

func TestStripCrossHostKeepsTheOriginAndDropsOnDowngrade(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, first, next string
		kept              bool
	}{
		{"host case", "https://API.example/x", "https://api.example/y", true},
		{"explicit default port", "https://api.example:443/x", "https://api.example/y", true},
		{"upgrade", "http://api.example/x", "https://api.example/y", true},
		{"downgrade", "https://api.example/x", "http://api.example/y", false},
		{"another port", "https://api.example/x", "https://api.example:8443/y", false},
	} {
		req, via := chain(t, c.first, c.next)
		req.Header = http.Header{"X-Api-Key": {"k"}}
		_ = httpkit.StripCrossHost()(req, via)
		if got := req.Header.Get("X-Api-Key") != ""; got != c.kept {
			t.Errorf("%s: kept = %v", c.name, got)
		}
	}
}

func TestStripCrossHostDropsANonCanonicalKey(t *testing.T) {
	t.Parallel()
	req, via := chain(t, "https://a.example/", "https://b.example/")
	req.Header = http.Header{"x-api-key": {"k"}, "x-customer": {"c"}}
	_ = httpkit.StripCrossHost("X-Customer")(req, via)
	if len(req.Header) != 0 {
		t.Fatalf("headers crossed hosts: %v", req.Header)
	}
}

// net/http copies the first request's headers onto every hop, so a chain that comes back to the origin would hand the
// credentials to it again after a hop elsewhere saw the chain; once the chain leaves, they stay stripped.
func TestStripCrossHostKeepsStrippingAfterTheChainLeaves(t *testing.T) {
	t.Parallel()
	for _, urls := range [][]string{
		{"https://a.example/", "https://b.example/", "https://a.example/"},
		{"https://a.example/", "http://a.example/", "https://a.example/"},
	} {
		req, via := chain(t, urls...)
		req.Header = http.Header{"X-Api-Key": {"k"}, "X-Customer": {"c"}}
		_ = httpkit.StripCrossHost("X-Customer")(req, via)
		if len(req.Header) != 0 {
			t.Errorf("%v: headers restored on the way back: %v", urls, req.Header)
		}
	}
}

func TestRedactURLAndError(t *testing.T) {
	if got := httpkit.RedactURL("https://u:p@h.example/a?api_key=SECRET#f"); got != "https://h.example/a" {
		t.Fatalf("RedactURL = %q", got)
	}
	_, err := httpkit.DoCapped(nil, mustReq(t, "http://127.0.0.1:1/x?api_key=SECRET"), 1, nil)
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("error leaks the query: %v", err)
	}
}

func mustReq(t *testing.T, u string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, u, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}
