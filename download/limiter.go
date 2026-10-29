package download

import (
	"context"
	"io"
	"sync"
	"time"
)

// minBurst is the bytes a download may take at once however low the limit is.
const minBurst = 16 << 10

// Limiter is one token bucket shared by every File that holds it. Rate is read on every chunk, so a change reaches
// downloads already running; nil or <= 0 is unlimited. A quarter second of the rate (at least 16 KiB) is available
// at once.
type Limiter struct {
	Rate func() int64 // Most bytes a second all holders together may use.

	mu     sync.Mutex
	tokens float64
	last   time.Time
}

// reserve takes n bytes from the bucket at rate bytes a second and says how long the caller must wait before
// sending them on.
func (l *Limiter) reserve(n, rate int64, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	burst := max(float64(rate)/4, minBurst)
	if l.last.IsZero() {
		l.tokens = burst
	} else {
		l.tokens = min(burst, l.tokens+now.Sub(l.last).Seconds()*float64(rate))
	}
	l.last = now
	l.tokens -= float64(n)
	if l.tokens >= 0 {
		return 0
	}
	return time.Duration(-l.tokens / float64(rate) * float64(time.Second))
}

// wrap throttles r through l; a nil l leaves it alone.
func (l *Limiter) wrap(ctx context.Context, r io.Reader) io.Reader {
	if l == nil || l.Rate == nil {
		return r
	}
	return &throttled{ctx: ctx, l: l, r: r}
}

type throttled struct {
	ctx context.Context //nolint:containedctx // an io.Reader has no context parameter; the reader lives for one copy
	l   *Limiter
	r   io.Reader
}

func (t *throttled) Read(p []byte) (int, error) {
	rate := t.l.Rate()
	if rate <= 0 {
		return t.r.Read(p)
	}
	if burst := max(rate/4, minBurst); int64(len(p)) > burst {
		p = p[:burst]
	}
	n, err := t.r.Read(p)
	if n > 0 {
		if wait := t.l.reserve(int64(n), rate, time.Now()); wait > 0 {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-t.ctx.Done():
				return n, t.ctx.Err()
			}
		}
	}
	return n, err
}
