package download

import (
	"context"
	"crypto/md5" // #nosec G501 -- Content-MD5 is MD5 by RFC 1864.
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	safefs "github.com/Rethunk-Tech/go-safefs"

	"github.com/Rethunk-Tech/go-httpkit"
)

const bigLimit = 1 << 20

func fetch(t *testing.T, srv *httptest.Server, dest string, opts Options) error {
	t.Helper()
	if opts.Limit == 0 {
		opts.Limit = bigLimit
	}
	return File(t.Context(), srv.Client(), srv.URL, dest, opts)
}

func TestFileRefusesABodyOverTheCap(t *testing.T) {
	t.Parallel()
	srv := bodyServer(t, "data", nil)
	dest := newDest(t)
	if err := fetch(t, srv, dest, Options{Limit: 3}); !errors.Is(err, safefs.ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
	if err := fetch(t, srv, dest, Options{Limit: -1}); err == nil {
		t.Fatal("a missing Limit must be refused")
	}
}

func TestFileResumesFromThePartialWithIfRange(t *testing.T) {
	t.Parallel()
	var seen requests
	srv := rangeServer(t, resumeBody, `"v1"`, &seen)
	dest := newDest(t)
	seedPartial(t, dest, resumeBody[:10], sidecar{URL: srv.URL, ETag: `"v1"`, ExpectedSize: int64(len(resumeBody))})
	if err := fetch(t, srv, dest, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, dest); got != resumeBody || len(seen.get()) != 1 || seen.get()[0] != `bytes=10-|"v1"` {
		t.Fatalf("got %q requests %v", got, seen.get())
	}
}

func TestFileResumesAV038Sidecar(t *testing.T) {
	t.Parallel()
	var seen requests
	srv := rangeServer(t, resumeBody, `"v1"`, &seen)
	dest := newDest(t)
	fixture, err := os.ReadFile("testdata/sidecar-v0.3.8.json")
	if err != nil {
		t.Fatal(err)
	}
	aimed := strings.ReplaceAll(string(fixture), "https://example.invalid/mod.zip", srv.URL)
	if err := safefs.WriteFile(Sidecar(dest), []byte(aimed), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte(resumeBody[:10]), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Partial(dest); got != 10 {
		t.Fatalf("Partial = %d", got)
	}
	if err := fetch(t, srv, dest, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, dest); got != resumeBody || len(seen.get()) != 1 || seen.get()[0] != `bytes=10-|"v1"` {
		t.Fatalf("got %q requests %v", got, seen.get())
	}
}

func TestFileStartsOverWhenTheValidatorChanged(t *testing.T) {
	t.Parallel()
	var seen requests
	srv := rangeServer(t, resumeBody, `"v2"`, &seen)
	dest := newDest(t)
	seedPartial(t, dest, "STALEPARTI", sidecar{URL: srv.URL, ETag: `"v1"`, ExpectedSize: int64(len(resumeBody))})
	if err := fetch(t, srv, dest, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, dest); got != resumeBody {
		t.Fatalf("got %q", got)
	}
}

func TestFileStartsOverWhenTheServerIgnoresRange(t *testing.T) {
	t.Parallel()
	srv := bodyServer(t, resumeBody, nil)
	dest := newDest(t)
	seedPartial(t, dest, "STALEPARTI", sidecar{URL: srv.URL, ExpectedSize: int64(len(resumeBody)), LastModified: "x"})
	if err := fetch(t, srv, dest, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, dest); got != resumeBody {
		t.Fatalf("got %q", got)
	}
}

func TestFileResumesOnANewAddressOfTheSameSize(t *testing.T) {
	t.Parallel()
	var seen requests
	srv := rangeServer(t, resumeBody, `"other-cdn"`, &seen)
	dest := newDest(t)
	seedPartial(t, dest, resumeBody[:10], sidecar{URL: "https://old.example/f", ETag: `"v1"`, ExpectedSize: int64(len(resumeBody))})
	if err := fetch(t, srv, dest, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, dest); got != resumeBody || len(seen.get()) != 1 || seen.get()[0] != "bytes=10-|" {
		t.Fatalf("got %q requests %v", got, seen.get())
	}
}

func TestFileStartsOverWhenTheTotalChanged(t *testing.T) {
	t.Parallel()
	var seen requests
	srv := rangeServer(t, resumeBody+"MORE", `"v1"`, &seen)
	dest := newDest(t)
	seedPartial(t, dest, resumeBody[:10], sidecar{URL: "https://old.example/f", ExpectedSize: int64(len(resumeBody))})
	if err := fetch(t, srv, dest, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, dest); got != resumeBody+"MORE" {
		t.Fatalf("got %q", got)
	}
}

func TestFileDropsAPartialNothingVouchesFor(t *testing.T) {
	t.Parallel()
	var seen requests
	srv := rangeServer(t, resumeBody, `"v1"`, &seen)
	dest := newDest(t)
	if err := os.WriteFile(dest, []byte("unknown"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fetch(t, srv, dest, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, dest); got != resumeBody || seen.get()[0] != "|" {
		t.Fatalf("got %q requests %v", got, seen.get())
	}
}

func TestFileVerifiesAPartialThatIsAlreadyComplete(t *testing.T) {
	t.Parallel()
	var seen requests
	srv := rangeServer(t, resumeBody, `"v1"`, &seen)
	dest := newDest(t)
	seedPartial(t, dest, resumeBody, sidecar{URL: srv.URL, ETag: `"v1"`, ExpectedSize: int64(len(resumeBody)), Hash: "sha256:" + strings.Repeat("0", 64)})
	if err := fetch(t, srv, dest, Options{}); !errors.Is(err, ErrChecksum) {
		t.Fatalf("err = %v", err)
	}
	if len(seen.get()) != 0 {
		t.Fatalf("a complete partial asked the server again: %v", seen.get())
	}
}

// A 200 describes a new body: what the sidecar recorded about the old one must not survive it.
func TestFileForgetsTheOldSizeWhenTheServerAnswers200(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = http.NewResponseController(w).Flush() // Chunked: no Content-Length.
		_, _ = w.Write([]byte(resumeBody + "-changed"))
	}))
	t.Cleanup(srv.Close)
	for _, partial := range []string{"STALEPARTI", ""} {
		dest := newDest(t)
		if partial != "" {
			seedPartial(t, dest, partial, sidecar{URL: srv.URL, ExpectedSize: int64(len(resumeBody))})
		} else {
			saveSidecar(dest, sidecar{URL: srv.URL, ExpectedSize: int64(len(resumeBody))})
		}
		if err := fetch(t, srv, dest, Options{}); err != nil {
			t.Fatalf("partial %q: %v", partial, err)
		}
		if got := readFile(t, dest); got != resumeBody+"-changed" {
			t.Fatalf("got %q", got)
		}
	}
}

func TestFileGivesUpOnALoopingPartialRange(t *testing.T) {
	t.Parallel()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n.Add(1)
		w.Header().Set("Content-Range", "bytes 1-9/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("0123456789"))
	}))
	t.Cleanup(srv.Close)
	dest := newDest(t)
	seedPartial(t, dest, "PARTIAL!!", sidecar{URL: srv.URL, ETag: `"v1"`, ExpectedSize: 10})
	if err := fetch(t, srv, dest, Options{}); !errors.Is(err, ErrNoResume) {
		t.Fatalf("err = %v", err)
	}
	if got := n.Load(); got != restarts+1 {
		t.Fatalf("requests = %d, want %d", got, restarts+1)
	}
}

func TestFileRestartsOnARangeThatIsNotAnInterval(t *testing.T) {
	t.Parallel()
	for _, cr := range []string{"bytes 10-5/20", "bytes 10-19/15"} {
		var n atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if n.Add(1) == 1 && r.Header.Get("Range") != "" {
				w.Header().Set("Content-Range", cr)
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte(resumeBody[10:]))
				return
			}
			_, _ = w.Write([]byte(resumeBody))
		}))
		dest := newDest(t)
		seedPartial(t, dest, resumeBody[:10], sidecar{URL: srv.URL, ETag: `"v1"`, ExpectedSize: int64(len(resumeBody))})
		err := File(t.Context(), srv.Client(), srv.URL, dest, Options{Limit: bigLimit})
		srv.Close()
		if err != nil || readFile(t, dest) != resumeBody {
			t.Fatalf("%s: %v %q", cr, err, readFile(t, dest))
		}
	}
}

func TestFileVerifiesChecksums(t *testing.T) {
	t.Parallel()
	sha := sha256.Sum256([]byte(resumeBody))
	sum := md5.Sum([]byte(resumeBody)) // #nosec G401 -- Content-MD5 is MD5 by RFC 1864.
	b64 := base64.StdEncoding.EncodeToString
	for _, c := range []struct {
		name    string
		header  map[string]string
		wantErr string
	}{
		{"sha256 matches", map[string]string{"X-Checksum-Sha256": hex.EncodeToString(sha[:])}, ""},
		{"sha256 differs", map[string]string{"X-Checksum-Sha256": strings.Repeat("0", 64)}, "sha256"},
		{"md5 matches", map[string]string{"Content-MD5": b64(sum[:])}, ""},
		{"md5 differs", map[string]string{"Content-MD5": b64(make([]byte, md5.Size))}, "Content-MD5"},
		{"md5 is not base64", map[string]string{"Content-MD5": "!!"}, "Content-MD5"},
		{"none announced", nil, ""},
	} {
		srv := bodyServer(t, resumeBody, c.header)
		dest := newDest(t)
		err := fetch(t, srv, dest, Options{})
		if c.wantErr == "" {
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
			}
			continue
		}
		if !errors.Is(err, ErrChecksum) || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("%s: err = %v", c.name, err)
		}
		if _, stat := os.Stat(dest); !os.IsNotExist(stat) {
			t.Errorf("%s: a corrupt file stayed on disk", c.name)
		}
		if _, stat := os.Stat(Sidecar(dest)); !os.IsNotExist(stat) {
			t.Errorf("%s: its sidecar stayed", c.name)
		}
	}
}

func TestFileFailsOnTheWrongSize(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-4/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("abcde"))
	}))
	t.Cleanup(srv.Close)
	dest := newDest(t)
	if err := fetch(t, srv, dest, Options{}); !errors.Is(err, ErrSize) {
		t.Fatalf("err = %v", err)
	}
	if Partial(dest) != 0 {
		t.Fatal("a wrong-sized file stayed on disk")
	}
}

func TestFileStatuses(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		code int
		want error
	}{
		{http.StatusUnauthorized, ErrLinkExpired},
		{http.StatusForbidden, ErrLinkExpired},
		{http.StatusGone, ErrLinkExpired},
		{http.StatusNotFound, httpkit.ErrNotFound},
		{http.StatusBadGateway, nil},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(c.code) }))
		err := fetch(t, srv, newDest(t), Options{})
		srv.Close()
		var se *httpkit.StatusError
		switch {
		case c.want != nil && !errors.Is(err, c.want):
			t.Errorf("%d: err = %v", c.code, err)
		case c.want == nil && (!errors.As(err, &se) || se.Code != c.code):
			t.Errorf("%d: err = %v", c.code, err)
		}
	}
}

func TestFileCheckSeesTheResponseFirstAndAuthorizeEveryRequest(t *testing.T) {
	t.Parallel()
	var auth atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "token" {
			auth.Add(1)
		}
		w.Header().Set("X-Limit", "yes")
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	limited := errors.New("host's own limit")
	err := fetch(t, srv, newDest(t), Options{
		Authorize: func(r *http.Request) { r.Header.Set("Authorization", "token") },
		Check: func(r *http.Response) error {
			if r.Header.Get("X-Limit") == "yes" {
				return limited
			}
			return nil
		},
	})
	if !errors.Is(err, limited) || auth.Load() != 1 {
		t.Fatalf("err = %v, authorized %d", err, auth.Load())
	}
}

func TestFileReportsProgress(t *testing.T) {
	t.Parallel()
	srv := bodyServer(t, resumeBody, nil)
	var done, total int64
	err := fetch(t, srv, newDest(t), Options{Progress: func(d, tot int64) { done, total = d, tot }})
	if err != nil || done != int64(len(resumeBody)) || total != int64(len(resumeBody)) {
		t.Fatalf("err %v, progress %d/%d", err, done, total)
	}
}

func TestFileRefusesAnHTTPSDowngrade(t *testing.T) {
	t.Parallel()
	plain := bodyServer(t, "secret", nil)
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	t.Cleanup(tls.Close)
	err := File(t.Context(), tls.Client(), tls.URL, newDest(t), Options{Limit: bigLimit})
	if err == nil || !strings.Contains(err.Error(), "refusing a redirect") {
		t.Fatalf("err = %v", err)
	}
}

func TestFileCancelKeepsThePartial(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("part"))
		_ = http.NewResponseController(w).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	dest := newDest(t)
	time.AfterFunc(100*time.Millisecond, cancel)
	err := File(ctx, srv.Client(), srv.URL, dest, Options{Limit: bigLimit, Idle: -1})
	if !errors.Is(err, context.Canceled) || Partial(dest) != 4 {
		t.Fatalf("err = %v, partial %d", err, Partial(dest))
	}
	Remove(dest)
	if Partial(dest) != 0 {
		t.Fatal("Remove left the pair")
	}
}
