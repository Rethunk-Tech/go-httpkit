package download

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	safefs "github.com/Rethunk-Tech/go-safefs"
)

const resumeBody = "abcdefghij0123456789"

// requests collects the Range and If-Range of each request a test server sees.
type requests struct {
	mu   sync.Mutex
	list []string
}

func (r *requests) add(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.list = append(r.list, req.Header.Get("Range")+"|"+req.Header.Get("If-Range"))
}

func (r *requests) get() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.list...)
}

// rangeServer honours Range from the start of body (a validator mismatch in If-Range answers the whole body, as
// HTTP says).
func rangeServer(t *testing.T, body, etag string, seen *requests) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.add(r)
		w.Header().Set("ETag", etag)
		var from int
		_, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-", &from)
		if err != nil || (r.Header.Get("If-Range") != "" && r.Header.Get("If-Range") != etag) {
			_, _ = io.WriteString(w, body)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(body)-1, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, body[from:])
	}))
	t.Cleanup(srv.Close)
	return srv
}

func bodyServer(t *testing.T, body string, header map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for k, v := range header {
			w.Header().Set(k, v)
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func seedPartial(t *testing.T, dest, partial string, m sidecar) {
	t.Helper()
	if err := os.WriteFile(dest, []byte(partial), 0o600); err != nil {
		t.Fatal(err)
	}
	saveSidecar(dest, m)
}

func newDest(t *testing.T) string { return filepath.Join(t.TempDir(), "m.zip") }

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := safefs.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
