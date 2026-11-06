package httpkit_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	safefs "github.com/Rethunk-Tech/go-safefs"

	"github.com/Rethunk-Tech/go-httpkit"
)

func TestDoJSON(t *testing.T) {
	t.Parallel()
	run := func(t *testing.T, h http.HandlerFunc, o httpkit.JSONOptions) (map[string]int, error) {
		t.Helper()
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]int
		err = httpkit.DoJSON(srv.Client(), req, &out, o)
		return out, err
	}
	reply := func(code int, body string, hdr ...string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Accept") != "application/json" {
				t.Errorf("Accept = %q", r.Header.Get("Accept"))
			}
			for i := 0; i+1 < len(hdr); i += 2 {
				w.Header().Set(hdr[i], hdr[i+1])
			}
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		}
	}

	t.Run("decodes a 200", func(t *testing.T) {
		t.Parallel()
		out, err := run(t, reply(200, `{"a":1}`), httpkit.JSONOptions{})
		if err != nil || out["a"] != 1 {
			t.Fatalf("out = %v, err = %v", out, err)
		}
	})
	t.Run("429 is a rate limit with its reset", func(t *testing.T) {
		t.Parallel()
		_, err := run(t, reply(429, `{}`, "Retry-After", "120"), httpkit.JSONOptions{})
		rl, ok := errors.AsType[*httpkit.RateLimitError](err)
		if !ok || rl.Reset.IsZero() || !errors.Is(err, httpkit.ErrRateLimited) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("other statuses are a StatusError", func(t *testing.T) {
		t.Parallel()
		_, err := run(t, reply(404, `{}`), httpkit.JSONOptions{})
		se, ok := errors.AsType[*httpkit.StatusError](err)
		if !ok || se.Code != 404 || !errors.Is(err, httpkit.ErrNotFound) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("Check claims a 403 before the status test", func(t *testing.T) {
		t.Parallel()
		mine := errors.New("mine")
		_, err := run(t, reply(403, `{}`), httpkit.JSONOptions{Check: func(r *http.Response) error {
			if r.StatusCode == http.StatusForbidden {
				return mine
			}
			return nil
		}})
		if !errors.Is(err, mine) {
			t.Fatalf("err = %v", err)
		}
		_, err = run(t, reply(403, `{}`), httpkit.JSONOptions{})
		if se, ok := errors.AsType[*httpkit.StatusError](err); !ok || se.Code != 403 {
			t.Fatalf("unclaimed 403: %v", err)
		}
	})
	t.Run("body over the limit", func(t *testing.T) {
		t.Parallel()
		_, err := run(t, reply(200, `{"a":1}`), httpkit.JSONOptions{Limit: 3})
		if !errors.Is(err, safefs.ErrTooLarge) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("malformed JSON", func(t *testing.T) {
		t.Parallel()
		if _, err := run(t, reply(200, `nope`), httpkit.JSONOptions{}); err == nil {
			t.Fatal("no error")
		}
	})
}
