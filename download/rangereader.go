package download

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

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
	etag     string // The first answer's strong ETag; later chunks must come from the same representation.
}

// NewRangeReader reads the size-byte file at url with hc (nil means a client with httpkit.DefaultTimeout).
func NewRangeReader(ctx context.Context, hc *http.Client, url string, size int64) *RangeReader {
	if hc == nil {
		hc = &http.Client{Timeout: httpkit.DefaultTimeout}
	}
	return &RangeReader{ctx: ctx, hc: httpkit.WithRedirect(hc, httpkit.NoDowngrade), url: url, size: size}
}

// ReadAt implements io.ReaderAt.
func (r *RangeReader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= r.size {
		return 0, io.EOF
	}
	end := r.at + int64(len(r.buf))
	// A chunk that ends at the file's end already holds every byte there is, however far p reaches past it.
	if off < r.at || (off+int64(len(p)) > end && end != r.size) {
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
		// The chunk was checked against its range, so a short copy means the file ends before off+len(p).
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
	if r.etag != "" {
		req.Header.Set("If-Range", r.etag)
	}
	resp, err := r.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ranged read: %w", httpkit.RedactError(err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusPartialContent {
		return nil, &httpkit.StatusError{Code: resp.StatusCode, Status: resp.Status}
	}
	// A server may answer a different range than asked, or end the body early; either would read as the file's end.
	// A total other than size, or an ETag other than the first chunk's, means the file changed between chunks.
	from, to, total, ok := parseContentRange(resp.Header.Get("Content-Range"))
	if !ok || from != start || to != end-1 || (total > 0 && total != r.size) {
		return nil, fmt.Errorf("ranged read: the answer is not bytes %d-%d of %d: %w", start, end-1, r.size, io.ErrUnexpectedEOF)
	}
	if etag := resp.Header.Get("ETag"); etag != "" && !strings.HasPrefix(etag, "W/") {
		if r.etag != "" && etag != r.etag {
			return nil, fmt.Errorf("ranged read: the file changed between chunks: %w", io.ErrUnexpectedEOF)
		}
		r.etag = etag
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, end-start))
	if err != nil {
		return nil, fmt.Errorf("ranged read: %w", httpkit.RedactError(err))
	}
	if int64(len(b)) != end-start {
		return nil, fmt.Errorf("ranged read: %d of %d bytes arrived: %w", len(b), end-start, io.ErrUnexpectedEOF)
	}
	return b, nil
}
