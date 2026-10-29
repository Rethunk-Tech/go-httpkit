package httpkit_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/Rethunk-Tech/go-httpkit"
)

type recorder struct {
	seen   *http.Request
	idle   bool
	err    error
	status int
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.seen = req
	if r.err != nil {
		return nil, r.err
	}
	return &http.Response{StatusCode: r.status, Request: req, Body: http.NoBody}, nil
}

func (r *recorder) CloseIdleConnections() { r.idle = true }

func TestTransportUserAgent(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, own, want string }{
		{"empty gets the default", "", "kit/1"},
		{"its own is kept", "mine/2", "mine/2"},
	} {
		base := &recorder{status: 200}
		tr := &httpkit.Transport{Base: base, UserAgent: "kit/1"}
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://x.invalid/", nil)
		if c.own != "" {
			req.Header.Set("User-Agent", c.own)
		}
		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if got := base.seen.Header.Get("User-Agent"); got != c.want {
			t.Errorf("%s: sent %q", c.name, got)
		}
		if req.Header.Get("User-Agent") != c.own {
			t.Errorf("%s: the caller's request was modified", c.name)
		}
	}
}

func TestTransportOnResultSeesBothOutcomes(t *testing.T) {
	t.Parallel()
	boom := errors.New("dial failed")
	for _, base := range []*recorder{{status: 204}, {err: boom}} {
		var gotResp *http.Response
		var gotErr error
		calls := 0
		tr := &httpkit.Transport{Base: base, OnResult: func(_ *http.Request, resp *http.Response, err error) {
			calls++
			gotResp, gotErr = resp, err
		}}
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://x.invalid/", nil)
		resp, err := tr.RoundTrip(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if calls != 1 || !errors.Is(err, gotErr) || resp != gotResp {
			t.Fatalf("calls %d, hook (%v, %v), returned (%v, %v)", calls, gotResp, gotErr, resp, err)
		}
		if base.err != nil && !errors.Is(gotErr, boom) {
			t.Fatalf("the hook got %v, want the transport's own error", gotErr)
		}
	}
}

func TestTransportCloseIdleConnectionsReachesBase(t *testing.T) {
	t.Parallel()
	base := &recorder{}
	(&httpkit.Transport{Base: base}).CloseIdleConnections()
	if !base.idle {
		t.Fatal("Base was not told")
	}
	(&httpkit.Transport{Base: roundTripOnly{}}).CloseIdleConnections() // Must not panic.
}

type roundTripOnly struct{}

func (roundTripOnly) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unused")
}
