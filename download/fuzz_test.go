package download

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestParseContentRange(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		in                string
		start, end, total int64
		ok                bool
	}{
		{"bytes 10-19/20", 10, 19, 20, true},
		{"bytes 0-0/1", 0, 0, 1, true},
		{"bytes 10-19/*", 10, 19, 0, true},
		{"bytes 10-5/20", 0, 0, 0, false},
		{"bytes 10-19/15", 0, 0, 0, false},
		{"bytes 10-19/19", 0, 0, 0, false},
		{"bytes -1-5/20", 0, 0, 0, false},
		{"bytes */20", 0, 0, 0, false},
		{"bytes 10/20", 0, 0, 0, false},
		{"bytes 10-19", 0, 0, 0, false},
		{"", 0, 0, 0, false},
	} {
		start, end, total, ok := parseContentRange(c.in)
		if ok != c.ok || start != c.start || end != c.end || total != c.total {
			t.Errorf("%q: got %d-%d/%d %v", c.in, start, end, total, ok)
		}
	}
}

func FuzzContentRange(f *testing.F) {
	for _, s := range []string{"bytes 10-19/20", "bytes 10-19/*", "bytes 10-5/20", "bytes 10-19/15", "", "bytes -1-2/3", "bytes 9223372036854775807-9223372036854775807/*"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, h string) {
		start, end, total, ok := parseContentRange(h)
		if ok && (start < 0 || start > end || (total != 0 && total <= end)) {
			t.Fatalf("%q accepted as %d-%d/%d", h, start, end, total)
		}
	})
}

// A sidecar is whatever a crash or another program left on disk: File must never panic, never leave more than Limit
// bytes, and never report success without a file.
func FuzzSidecar(f *testing.F) {
	fixture, err := os.ReadFile("testdata/sidecar.json")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(fixture, []byte(resumeBody[:10]))
	f.Add([]byte(`{"expectedSize":-5,"url":"x"}`), []byte("junk"))
	f.Add([]byte(`{"expectedSize":20,"hash":"sha256:00","md5":"%%"}`), []byte(resumeBody))
	f.Add([]byte("{"), []byte{})
	f.Add([]byte(`{"expectedSize":99999999999999999999}`), []byte("p"))
	const limit = 64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(resumeBody))
	}))
	f.Cleanup(srv.Close)
	f.Fuzz(func(t *testing.T, side, partial []byte) {
		dest := newDest(t)
		if err := os.WriteFile(Sidecar(dest), []byte(strings.ReplaceAll(string(side), "https://example.invalid/mod.zip", srv.URL)), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, partial, 0o600); err != nil {
			t.Fatal(err)
		}
		err := File(t.Context(), srv.Client(), srv.URL, dest, Options{Limit: limit})
		if st, statErr := os.Stat(dest); statErr == nil {
			if st.Size() > limit {
				t.Fatalf("left %d bytes over the limit", st.Size())
			}
		} else if err == nil {
			t.Fatal("succeeded without a file")
		}
	})
}
