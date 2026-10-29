package download

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/Rethunk-Tech/go-httpkit"
)

const (
	defaultChunk       = 64 << 10
	defaultMaxRequests = 8
)

// ErrRangeBudget is what RangeReader returns once it needs more ranged requests than it may make.
var ErrRangeBudget = errors.New("the remote read needed more ranged requests than allowed")

// RangeReader reads a remote file of known size through ranged GETs, keeping the last chunk it fetched. Past
// MaxRequests it fails with ErrRangeBudget; an answer other than 206 is a *httpkit.StatusError. It is an io.ReaderAt
// but not safe for concurrent use.
type RangeReader struct {
	MaxRequests int   // Zero means 8.
	Chunk       int64 // Zero means 64 KiB.

	ctx      context.Context //nolint:containedctx // io.ReaderAt has no context parameter
	hc       *http.Client
	url      string
	size     int64
	at       int64
	buf      []byte
	requests int
}

// NewRangeReader reads the size-byte file at url with hc (nil means http.DefaultClient).
func NewRangeReader(ctx context.Context, hc *http.Client, url string, size int64) *RangeReader {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &RangeReader{ctx: ctx, hc: hc, url: url, size: size}
}

// ReadAt implements io.ReaderAt.
func (r *RangeReader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= r.size {
		return 0, io.EOF
	}
	if off < r.at || off+int64(len(p)) > r.at+int64(len(r.buf)) {
		r.requests++
		if r.requests > cmp.Or(r.MaxRequests, defaultMaxRequests) {
			return 0, ErrRangeBudget
		}
		// A zip's directory is read from its end backwards, so a chunk ending at the file's end serves most reads.
		n := max(int64(len(p)), cmp.Or(r.Chunk, defaultChunk))
		start := min(off, max(0, r.size-n))
		b, err := r.get(start, min(r.size, start+n))
		if err != nil {
			return 0, err
		}
		r.at, r.buf = start, b
	}
	if off < r.at || off-r.at > int64(len(r.buf)) {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, r.buf[off-r.at:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// get fetches bytes [start, end).
func (r *RangeReader) get(start, end int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return nil, fmt.Errorf("ranged read: %w", err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end-1))
	resp, err := r.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ranged read: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusPartialContent {
		return nil, &httpkit.StatusError{Code: resp.StatusCode, Status: resp.Status}
	}
	return io.ReadAll(io.LimitReader(resp.Body, end-start))
}
