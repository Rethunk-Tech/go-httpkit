package download

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// stalls answers with the first bytes of a body and then sends nothing until the client hangs up.
func stalls(part string, length int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(length))
		_, _ = io.WriteString(w, part)
		_ = http.NewResponseController(w).Flush()
		<-r.Context().Done()
	})
}

// whole answers every request with body.
func whole(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) })
}

func TestFileStallFailsWithErrIdleAndKeepsThePartial(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		srv := httptest.NewTestServer(t, stalls("partial", 100))
		dest := newDest(t)
		start := time.Now()
		err := File(t.Context(), srv.Client(), srv.URL, dest, Options{Limit: bigLimit})
		if !errors.Is(err, ErrIdle) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v", err)
		}
		if took := time.Since(start); took != defaultIdle {
			t.Fatalf("gave up after %v, want %v", took, defaultIdle)
		}
		if Partial(dest) != int64(len("partial")) {
			t.Fatalf("the partial is %d bytes", Partial(dest))
		}
	})
}

func TestFileServerThatNeverAnswersFailsWithErrIdle(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		srv := httptest.NewTestServer(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
		start := time.Now()
		err := File(t.Context(), srv.Client(), srv.URL, newDest(t), Options{Limit: bigLimit})
		if !errors.Is(err, ErrIdle) || time.Since(start) != defaultIdle {
			t.Fatalf("err = %v after %v", err, time.Since(start))
		}
	})
}

func TestFileIdleOptionSetsAndDisablesTheLimit(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		srv := httptest.NewTestServer(t, stalls("x", 100))
		start := time.Now()
		err := File(t.Context(), srv.Client(), srv.URL, newDest(t), Options{Limit: bigLimit, Idle: 5 * time.Second})
		if !errors.Is(err, ErrIdle) || time.Since(start) != 5*time.Second {
			t.Fatalf("err = %v after %v", err, time.Since(start))
		}

		ctx, cancel := context.WithTimeout(t.Context(), time.Hour)
		defer cancel()
		err = File(ctx, srv.Client(), srv.URL, newDest(t), Options{Limit: bigLimit, Idle: -1})
		if errors.Is(err, ErrIdle) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("a disabled limit still fired: %v", err)
		}
	})
}

// A slow disk or progress callback is not a stall: only a blocked body Read counts.
func TestFileSlowConsumerIsNotIdle(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		srv := httptest.NewTestServer(t, whole("0123456789"))
		err := File(t.Context(), srv.Client(), srv.URL, newDest(t), Options{
			Limit:    bigLimit,
			Idle:     time.Second,
			Progress: func(int64, int64) { time.Sleep(10 * time.Second) },
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

// Over HTTP/2 the body Read returns a bare context.Canceled, so the cause must be read from the request context.
func TestFileStallOverHTTP2(t *testing.T) {
	t.Parallel()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Errorf("served over HTTP/%d", r.ProtoMajor)
		}
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "part")
		_ = http.NewResponseController(w).Flush()
		<-r.Context().Done()
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	dest := newDest(t)
	err := File(t.Context(), srv.Client(), srv.URL, dest, Options{Limit: bigLimit, Idle: 200 * time.Millisecond})
	if !errors.Is(err, ErrIdle) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	if Partial(dest) != 4 {
		t.Fatalf("partial = %d", Partial(dest))
	}
}

func TestFileLimiterThrottlesInVirtualTime(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		body := strings.Repeat("x", 120<<10)
		srv := httptest.NewTestServer(t, whole(body))
		l := &Limiter{Rate: func() int64 { return 200 << 10 }}
		dest := newDest(t)
		start := time.Now()
		if err := File(t.Context(), srv.Client(), srv.URL, dest, Options{Limit: bigLimit, Limiter: l}); err != nil {
			t.Fatal(err)
		}
		// 120 KiB at 200 KiB/s with a 50 KiB burst leaves 70 KiB: 0.35 s.
		if took := time.Since(start); took < 300*time.Millisecond || took > time.Second {
			t.Fatalf("120 KiB arrived in %v under a 200 KiB/s limit", took)
		}
		if readFile(t, dest) != body {
			t.Fatal("a limited download is corrupt")
		}
	})
}

// The wiring of File to a Limiter over a real socket; the timing is asserted above in virtual time.
func TestFileHoldsTheLimiterOverARealSocket(t *testing.T) {
	t.Parallel()
	body := strings.Repeat("x", 40<<10)
	srv := bodyServer(t, body, nil)
	l := &Limiter{Rate: func() int64 { return 100 << 10 }}
	start := time.Now()
	if err := fetch(t, srv, newDest(t), Options{Limiter: l}); err != nil {
		t.Fatal(err)
	}
	// 40 KiB at 100 KiB/s with a 25 KiB burst leaves 15 KiB: 0.15 s.
	if took := time.Since(start); took < 100*time.Millisecond {
		t.Fatalf("40 KiB arrived in %v", took)
	}
}

func TestLimiterSharesTheRateAcrossCallers(t *testing.T) {
	t.Parallel()
	var l Limiter
	now := time.Unix(1_000, 0)
	const rate = 100 << 10
	if w := l.reserve(rate/4, rate, now); w != 0 {
		t.Fatalf("the burst waited %v", w)
	}
	// A second download draws on the same, now empty, bucket.
	if w := l.reserve(rate/4, rate, now); w != 250*time.Millisecond {
		t.Fatalf("second caller waits %v, want 250ms", w)
	}
	// Raising the limit at once shortens the wait for the same bytes.
	if w := l.reserve(rate/4, 2*rate, now); w >= 250*time.Millisecond*2 {
		t.Fatalf("a raised limit still waits %v", w)
	}
}

func TestLimiterNilAndNonPositiveRateAreUnlimited(t *testing.T) {
	t.Parallel()
	r := strings.NewReader("abc")
	if (*Limiter)(nil).wrap(t.Context(), r) != io.Reader(r) || (&Limiter{}).wrap(t.Context(), r) != io.Reader(r) {
		t.Fatal("an unlimited Limiter wrapped the reader")
	}
	got, err := io.ReadAll((&Limiter{Rate: func() int64 { return 0 }}).wrap(t.Context(), r))
	if err != nil || string(got) != "abc" {
		t.Fatalf("%q, %v", got, err)
	}
}
