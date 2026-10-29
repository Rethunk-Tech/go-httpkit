package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var (
	// ErrLinkExpired is a host refusing this URL with 401, 403 or 410; the caller may fetch a fresh link.
	ErrLinkExpired = errors.New("the download link has expired")
	// ErrChecksum is a finished file that does not match a digest the server announced; the file is removed.
	ErrChecksum = errors.New("the download did not match its checksum")
	// ErrSize is a finished file whose size is not the one the server announced; the file is removed.
	ErrSize = errors.New("the download is not the size the server announced")
	// ErrNoResume is a server that kept answering with a range File cannot resume from.
	ErrNoResume = errors.New("the server kept answering with a range that cannot resume")
	// ErrIdle is a body that sent nothing for Options.Idle. It matches context.DeadlineExceeded.
	ErrIdle = fmt.Errorf("no data arrived within the idle limit: %w", context.DeadlineExceeded)
)

// errNoLimit is a File call without the size bound it exists to enforce.
var errNoLimit = errors.New("download: Options.Limit must be positive")

const (
	// restarts is how many times File starts over after a response it cannot resume from.
	restarts = 3
	// defaultIdle is Options.Idle's zero value.
	defaultIdle = 30 * time.Second
)

// Progress reports bytes received so far and the expected total (0 when unknown).
type Progress func(done, total int64)

// Options tune File. The zero value of every field but Limit is the default.
type Options struct {
	Limit     int64                      // Required, > 0: most bytes the file may hold.
	Progress  Progress                   // Called after every write.
	Idle      time.Duration              // Zero means 30 s; negative disables. One body Read blocked this long fails with ErrIdle.
	Limiter   *Limiter                   // Nil means unthrottled.
	Authorize func(*http.Request)        // Sets credentials; called on every attempt's request.
	Check     func(*http.Response) error // Runs before the status switch; non-nil aborts (a host's own limit answer).
}

// idle is the effective idle limit; zero means none.
func (o Options) idle() time.Duration {
	switch {
	case o.Idle < 0:
		return 0
	case o.Idle == 0:
		return defaultIdle
	}
	return o.Idle
}

type progressWriter struct {
	w           io.Writer
	done, total int64
	fn          Progress
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	p.fn(p.done, p.total)
	return n, err
}
