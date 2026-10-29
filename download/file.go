package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	safefs "github.com/Rethunk-Tech/go-safefs"

	"github.com/Rethunk-Tech/go-httpkit"
)

// File streams url into dest. A partial dest with its sidecar is resumed with Range and If-Range when the server
// and the recorded validators allow; anything else starts over. Redirects follow httpkit.NoDowngrade. A nil hc
// means &http.Client{Timeout: 30 * time.Minute}; http.DefaultClient has no timeout at all.
//
// A body over opts.Limit fails with an error matching safefs.ErrTooLarge, any status other than 200, 206, 401, 403,
// 410 or 416 with a *httpkit.StatusError. A network error, stall or cancel keeps the partial for the next call; a
// size or checksum mismatch removes it.
func File(ctx context.Context, hc *http.Client, url, dest string, opts Options) error {
	if opts.Limit <= 0 {
		return errNoLimit
	}
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Minute}
	}
	x := &transfer{hc: httpkit.WithRedirect(hc, httpkit.NoDowngrade), url: url, dest: dest, opts: opts}
	for range restarts + 1 {
		retry, err := x.once(ctx)
		if !retry {
			return err
		}
		Remove(dest)
	}
	return ErrNoResume
}

// transfer is one File call; once runs an attempt, which prepare, accept and write divide between them.
type transfer struct {
	hc        *http.Client
	url, dest string
	opts      Options

	meta    sidecar
	sameURL bool  // The sidecar was written for this URL (or for none).
	offset  int64 // Bytes of the partial the request continues from.
}

// reset drops the partial and what was recorded about it.
func (x *transfer) reset() {
	Remove(x.dest)
	x.offset = 0
	x.meta = sidecar{URL: x.url}
}

// once makes one request. retry means the answer cannot be resumed from and the caller should start over.
func (x *transfer) once(ctx context.Context) (retry bool, err error) {
	if finished, err := x.prepare(); finished {
		return false, err
	}
	reqCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, x.url, nil)
	if err != nil {
		return false, fmt.Errorf("build the download request: %w", err)
	}
	if x.offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", x.offset))
		if v := x.meta.ifRange(); v != "" && x.sameURL {
			req.Header.Set("If-Range", v)
		}
	}
	if x.opts.Authorize != nil {
		x.opts.Authorize(req)
	}
	resp, err := x.hc.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	if x.opts.Check != nil {
		if err := x.opts.Check(resp); err != nil {
			return false, err
		}
	}
	if retry, err := x.accept(resp); retry || err != nil {
		return retry, err
	}
	return false, x.write(ctx, reqCtx, cancel, resp)
}

// prepare reads the sidecar and decides what to continue from. finished means there is nothing to request: the
// partial is already the whole file, and err is its verification.
func (x *transfer) prepare() (finished bool, err error) {
	x.meta = loadSidecar(x.dest)
	// A new address (another CDN, a fresh signed link) serves the same file under its own validators, so only the
	// size and the final checks vouch for it then.
	x.sameURL = x.meta.URL == "" || x.meta.URL == x.url
	if x.meta.URL == "" {
		x.meta.URL = x.url
	}
	x.offset = 0
	if st, err := safefs.Stat(x.dest); err == nil {
		x.offset = st.Size()
	}
	switch {
	case x.offset > x.opts.Limit:
		x.reset() // Written under a larger limit; the bound is checked against what is on disk too.
	case x.offset > 0 && x.meta.ExpectedSize == 0 && (!x.sameURL || x.meta.ifRange() == ""):
		x.reset() // Nothing vouches for these bytes.
	case x.meta.ExpectedSize > 0 && x.offset > x.meta.ExpectedSize:
		x.reset()
	case x.meta.ExpectedSize > 0 && x.offset == x.meta.ExpectedSize:
		return true, verify(x.dest, x.meta)
	}
	return false, nil
}

// accept applies the status and the validators to the plan. retry means the answer cannot be continued from.
func (x *transfer) accept(resp *http.Response) (retry bool, err error) {
	if retry, err := x.status(resp); retry || err != nil {
		return retry, err
	}
	if x.validators(resp) {
		return true, nil
	}
	x.meta.URL = x.url
	switch {
	case resp.StatusCode == http.StatusOK && resp.ContentLength > 0:
		x.meta.ExpectedSize = resp.ContentLength
	case resp.StatusCode == http.StatusPartialContent && x.meta.ExpectedSize == 0 && resp.ContentLength > 0:
		x.meta.ExpectedSize = x.offset + resp.ContentLength
	}
	if x.meta.ExpectedSize > x.opts.Limit || resp.ContentLength > x.opts.Limit {
		return false, x.tooLarge()
	}
	return false, nil
}

func (x *transfer) tooLarge() error {
	return fmt.Errorf("%w: the file may hold %d bytes", safefs.ErrTooLarge, x.opts.Limit)
}

func (x *transfer) status(resp *http.Response) (retry bool, err error) {
	switch resp.StatusCode {
	case http.StatusOK:
		// Any 200 starts over: the old size and digests describe another body.
		if x.offset > 0 {
			Remove(x.dest)
			x.offset = 0
		}
		x.meta = sidecar{URL: x.url}
	case http.StatusPartialContent:
		start, _, total, ok := parseContentRange(resp.Header.Get("Content-Range"))
		if !ok || start != x.offset {
			return true, nil
		}
		if x.meta.ExpectedSize > 0 && total > 0 && total != x.meta.ExpectedSize {
			return true, nil
		}
		if total > 0 {
			x.meta.ExpectedSize = total
		}
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusGone:
		return false, ErrLinkExpired
	case http.StatusRequestedRangeNotSatisfiable:
		return true, nil
	default:
		return false, &httpkit.StatusError{Code: resp.StatusCode, Status: resp.Status}
	}
	return false, nil
}

// validators records the response's validators and digests; stale reports a 206 whose validator differs from the
// one the partial was written under.
func (x *transfer) validators(resp *http.Response) (stale bool) {
	resumed := x.offset > 0 && resp.StatusCode == http.StatusPartialContent
	if etag := resp.Header.Get("ETag"); etag != "" {
		if x.sameURL && x.meta.ETag != "" && x.meta.ETag != etag && resumed {
			return true
		}
		x.meta.ETag = etag
	}
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		if x.sameURL && x.meta.ETag == "" && x.meta.LastModified != "" && x.meta.LastModified != lm && resumed {
			return true
		}
		x.meta.LastModified = lm
	}
	if h := hashFrom(resp.Header); h != "" {
		x.meta.Hash = h
	}
	if v := strings.TrimSpace(resp.Header.Get("Content-MD5")); v != "" {
		x.meta.MD5 = v
	}
	return false
}

// write copies the body onto dest, then checks its size and digests. The sidecar is written before the first byte,
// so a crash mid-body leaves a resumable pair.
func (x *transfer) write(ctx, reqCtx context.Context, cancel context.CancelCauseFunc, resp *http.Response) error {
	var f *os.File
	var err error
	if x.offset > 0 {
		f, err = safefs.OpenFile(x.dest, os.O_WRONLY|os.O_APPEND, 0o600)
	} else {
		f, err = safefs.Create(x.dest)
	}
	if err != nil {
		return fmt.Errorf("open the download: %w", err)
	}
	defer func() { _ = f.Close() }()
	saveSidecar(x.dest, x.meta)

	var w io.Writer = f
	if x.opts.Progress != nil {
		w = &progressWriter{w: f, done: x.offset, total: x.meta.ExpectedSize, fn: x.opts.Progress}
	}
	var src io.Reader = resp.Body
	if idle := x.opts.idle(); idle > 0 {
		src = &idleBody{r: src, idle: idle, cancel: cancel}
	}
	src = x.opts.Limiter.wrap(ctx, safefs.CapReader(src, x.opts.Limit-x.offset))
	if _, err := io.Copy(w, src); err != nil {
		return x.copyFailed(ctx, reqCtx, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close the download: %w", err)
	}
	st, err := safefs.Stat(x.dest)
	if err != nil {
		return fmt.Errorf("measure the download: %w", err)
	}
	if x.meta.ExpectedSize > 0 && st.Size() != x.meta.ExpectedSize {
		Remove(x.dest)
		return fmt.Errorf("%w: %d bytes, not %d", ErrSize, st.Size(), x.meta.ExpectedSize)
	}
	return verify(x.dest, x.meta)
}

// copyFailed names why the body copy stopped. A cancel wins over whatever the transport reported, and a stall is
// read from the request's cancel cause because over HTTP/2 the body Read returns a bare context.Canceled.
func (x *transfer) copyFailed(ctx, reqCtx context.Context, err error) error {
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.Is(err, safefs.ErrTooLarge):
		return x.tooLarge()
	case errors.Is(context.Cause(reqCtx), ErrIdle):
		return ErrIdle
	}
	return fmt.Errorf("download %s: %w", x.url, err)
}

// idleBody cancels the request when one Read blocks for idle. The timer runs only inside Read, so disk writes and
// progress callbacks never count as a stall.
type idleBody struct {
	r      io.Reader
	idle   time.Duration
	cancel context.CancelCauseFunc
}

func (b *idleBody) Read(p []byte) (int, error) {
	t := time.AfterFunc(b.idle, func() { b.cancel(ErrIdle) })
	defer t.Stop()
	return b.r.Read(p)
}
