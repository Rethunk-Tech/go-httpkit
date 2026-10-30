package httpkit_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	safefs "github.com/Rethunk-Tech/go-safefs"

	"github.com/Rethunk-Tech/go-httpkit"
)

// trackedBody records Close so a test can prove DoCapped never leaks the connection.
type trackedBody struct {
	io.Reader
	closed atomic.Bool
}

func (b *trackedBody) Close() error { b.closed.Store(true); return nil }

type fakeTransport func(*http.Request) *http.Response

func (f fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r), nil }

func TestDoCapped(t *testing.T) {
	t.Parallel()
	get := func(srv *httptest.Server) *http.Request {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		return req
	}
	serve := func(code int, body string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Seen", "yes")
			w.WriteHeader(code)
			_, _ = io.WriteString(w, body)
		}))
		t.Cleanup(srv.Close)
		return srv
	}

	t.Run("a 200 under and at the cap", func(t *testing.T) {
		t.Parallel()
		srv := serve(200, "four")
		for _, limit := range []int64{4, 100} {
			got, err := httpkit.DoCapped(srv.Client(), get(srv), limit, nil)
			if err != nil || string(got) != "four" {
				t.Fatalf("limit %d: %q, %v", limit, got, err)
			}
		}
	})
	t.Run("a body over the cap", func(t *testing.T) {
		t.Parallel()
		srv := serve(200, "four")
		if _, err := httpkit.DoCapped(srv.Client(), get(srv), 3, nil); !errors.Is(err, safefs.ErrTooLarge) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("statuses match the sentinels", func(t *testing.T) {
		t.Parallel()
		for code, want := range map[int]error{404: httpkit.ErrNotFound, 429: httpkit.ErrRateLimited, 502: nil} {
			srv := serve(code, "x")
			_, err := httpkit.DoCapped(srv.Client(), get(srv), 10, nil)
			var se *httpkit.StatusError
			if !errors.As(err, &se) || se.Code != code {
				t.Fatalf("%d: err = %v", code, err)
			}
			if want != nil && !errors.Is(err, want) {
				t.Errorf("%d does not match %v", code, want)
			}
			if code == 502 && (errors.Is(err, httpkit.ErrNotFound) || errors.Is(err, httpkit.ErrRateLimited) || err.Error() != "server answered 502 Bad Gateway") {
				t.Errorf("502: %v", err)
			}
		}
	})
	t.Run("check sees a 403 first and its error wins", func(t *testing.T) {
		t.Parallel()
		srv := serve(403, "x")
		sentinel := errors.New("limit")
		var saw string
		_, err := httpkit.DoCapped(srv.Client(), get(srv), 10, func(r *http.Response) error {
			saw = r.Header.Get("X-Seen")
			return sentinel
		})
		if !errors.Is(err, sentinel) || saw != "yes" {
			t.Fatalf("err = %v, saw %q", err, saw)
		}
	})
	t.Run("the body is closed on every path", func(t *testing.T) {
		t.Parallel()
		for _, code := range []int{200, 500} {
			body := &trackedBody{Reader: strings.NewReader("data")}
			hc := &http.Client{Transport: fakeTransport(func(r *http.Request) *http.Response {
				return &http.Response{StatusCode: code, Status: "x", Body: body, Header: http.Header{}, Request: r}
			})}
			req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.invalid/", nil)
			_, _ = httpkit.DoCapped(hc, req, 10, nil)
			if !body.closed.Load() {
				t.Errorf("status %d left the body open", code)
			}
		}
	})
}

func TestRateLimitErrorMatches(t *testing.T) {
	t.Parallel()
	var err error = &httpkit.RateLimitError{}
	if !errors.Is(err, httpkit.ErrRateLimited) || errors.Is(err, httpkit.ErrNotFound) || err.Error() != "rate limited" {
		t.Fatalf("zero reset: %v", err)
	}
	at := parse(t, "Wed, 21 Oct 2026 07:28:00 GMT")
	if got := (&httpkit.RateLimitError{Reset: at}).Error(); got != "rate limited until 2026-10-21T07:28:00Z" {
		t.Fatalf("got %q", got)
	}
}

func TestStatusErrorMatchesOnlyItsOwnSentinel(t *testing.T) {
	e := &httpkit.StatusError{Code: 429, Status: "429 Too Many Requests"}
	if !errors.Is(e, httpkit.ErrRateLimited) || errors.Is(e, httpkit.ErrNotFound) || errors.Is(e, io.EOF) {
		t.Errorf("Is: %v", e)
	}
}
