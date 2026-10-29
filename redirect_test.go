package httpkit_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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
