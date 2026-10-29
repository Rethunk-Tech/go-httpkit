package download

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Rethunk-Tech/go-httpkit"
)

// partialServer answers each ranged GET with its slice of body and counts them.
func partialServer(t *testing.T, body string, n *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		var from, to int
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &from, &to); err != nil {
			http.Error(w, "range please", http.StatusBadRequest)
			return
		}
		to = min(to, len(body)-1)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, to, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, body[from:to+1])
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRangeReaderReadsTheTailFirstAndKeepsTheChunk(t *testing.T) {
	t.Parallel()
	body := strings.Repeat("0123456789", 20_000) // 200 KB.
	var n atomic.Int32
	srv := partialServer(t, body, &n)
	r := NewRangeReader(t.Context(), srv.Client(), srv.URL, int64(len(body)))

	p := make([]byte, 100)
	if _, err := r.ReadAt(p, int64(len(body))-100); err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if string(p) != body[len(body)-100:] {
		t.Fatal("the tail read the wrong bytes")
	}
	// Inside the chunk just fetched: no new request.
	if _, err := r.ReadAt(p, int64(len(body))-1000); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 1 {
		t.Fatalf("requests = %d, want 1", n.Load())
	}
	// Before it: a second request.
	if _, err := r.ReadAt(p, 0); err != nil || string(p) != body[:100] || n.Load() != 2 {
		t.Fatalf("err %v, requests %d", err, n.Load())
	}
	if _, err := r.ReadAt(p, int64(len(body))); !errors.Is(err, io.EOF) {
		t.Fatalf("past the end: %v", err)
	}
}

func TestRangeReaderBudgetAndStatus(t *testing.T) {
	t.Parallel()
	body := strings.Repeat("x", 1<<20)
	var n atomic.Int32
	srv := partialServer(t, body, &n)
	r := NewRangeReader(t.Context(), srv.Client(), srv.URL, int64(len(body)))
	r.MaxRequests, r.Chunk = 2, 1024
	p := make([]byte, 10)
	for _, off := range []int64{0, 5000} {
		if _, err := r.ReadAt(p, off); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.ReadAt(p, 90_000); !errors.Is(err, ErrRangeBudget) {
		t.Fatalf("err = %v", err)
	}
	if n.Load() != 2 {
		t.Fatalf("a refused read still asked the server: %d", n.Load())
	}

	whole := bodyServer(t, body, nil)
	_, err := NewRangeReader(t.Context(), whole.Client(), whole.URL, int64(len(body))).ReadAt(p, 0)
	var se *httpkit.StatusError
	if !errors.As(err, &se) || se.Code != http.StatusOK {
		t.Fatalf("a server ignoring Range: %v", err)
	}
}
