package download

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Rethunk-Tech/go-safefs"
)

// validators and copyFailed decide whether a resume is trusted and which error a broken copy reports; their branches
// are cheaper to drive directly than through a server that misbehaves on cue.
func TestValidatorsRefuseAResumeUnderAnotherValidator(t *testing.T) {
	hdr := func(kv map[string]string) http.Header {
		h := http.Header{}
		for k, v := range kv {
			h.Set(k, v)
		}
		return h
	}
	for name, c := range map[string]struct {
		meta   sidecar
		status int
		header http.Header
		stale  bool
	}{
		"etag changed":          {sidecar{ETag: `"a"`}, http.StatusPartialContent, hdr(map[string]string{"ETag": `"b"`}), true},
		"etag same":             {sidecar{ETag: `"a"`}, http.StatusPartialContent, hdr(map[string]string{"ETag": `"a"`}), false},
		"last-modified changed": {sidecar{LastModified: "Mon"}, http.StatusPartialContent, hdr(map[string]string{"Last-Modified": "Tue"}), true},
		"etag outranks date":    {sidecar{ETag: `"a"`, LastModified: "Mon"}, http.StatusPartialContent, hdr(map[string]string{"ETag": `"a"`, "Last-Modified": "Tue"}), false},
		"not a resume":          {sidecar{ETag: `"a"`}, http.StatusOK, hdr(map[string]string{"ETag": `"b"`}), false},
	} {
		x := &transfer{offset: 10, sameURL: true, meta: c.meta}
		if got := x.validators(&http.Response{StatusCode: c.status, Header: c.header}); got != c.stale {
			t.Errorf("%s: stale %v, want %v", name, got, c.stale)
		}
	}
	x := &transfer{}
	x.validators(&http.Response{StatusCode: http.StatusOK, Header: hdr(map[string]string{"Content-MD5": " abc= ", "ETag": `"e"`})})
	if x.meta.MD5 != "abc=" || x.meta.ETag != `"e"` {
		t.Errorf("recorded %+v", x.meta)
	}
}

func TestCopyFailedNamesTheCause(t *testing.T) {
	x := &transfer{url: "u", opts: Options{Limit: 5}}
	live := context.Background()
	cancelled, cancel := context.WithCancel(live)
	cancel()
	idle, stall := context.WithCancelCause(live)
	stall(ErrIdle)
	boom := errors.New("boom")
	if err := x.copyFailed(cancelled, live, boom); !errors.Is(err, context.Canceled) {
		t.Errorf("cancel: %v", err)
	}
	if err := x.copyFailed(live, live, safefs.ErrTooLarge); !errors.Is(err, safefs.ErrTooLarge) {
		t.Errorf("limit: %v", err)
	}
	if err := x.copyFailed(live, idle, boom); !errors.Is(err, ErrIdle) {
		t.Errorf("stall: %v", err)
	}
	if err := x.copyFailed(live, live, boom); !errors.Is(err, boom) {
		t.Errorf("other: %v", err)
	}
}

func TestNewRangeReaderDefaultsItsClient(t *testing.T) {
	if r := NewRangeReader(context.Background(), nil, "u", 1); r.hc != http.DefaultClient {
		t.Error("a nil client was kept")
	}
}
