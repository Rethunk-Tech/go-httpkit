package httpkit_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/Rethunk-Tech/go-httpkit"
)

func parse(t *testing.T, v string) time.Time {
	t.Helper()
	at, err := http.ParseTime(v)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func TestRetryAfter(t *testing.T) {
	t.Parallel()
	const date = "Wed, 21 Oct 2026 07:28:00 GMT"
	past := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)
	for _, c := range []struct {
		in   string
		ok   bool
		near time.Duration // Expected distance from now; ignored for dates.
	}{
		{"120", true, 120 * time.Second},
		{"31536000", true, 365 * 24 * time.Hour},
		{"0", false, 0},
		{"-5", false, 0},
		{"31536001", false, 0},
		{"9223372037", false, 0},
		{"27670116110", false, 0},
		{"garbage", false, 0},
		{"", false, 0},
		{date, true, 0},
		{past, true, 0},
	} {
		at, ok := httpkit.RetryAfter(c.in)
		if ok != c.ok {
			t.Errorf("%q: ok = %v", c.in, ok)
			continue
		}
		switch {
		case !ok:
			if !at.IsZero() {
				t.Errorf("%q: refused but returned %v", c.in, at)
			}
		case c.near > 0:
			if d := time.Until(at); d > c.near || d < c.near-5*time.Second {
				t.Errorf("%q: %v from now, want about %v", c.in, d, c.near)
			}
		case c.in == date:
			if !at.Equal(parse(t, date)) {
				t.Errorf("date: %v", at)
			}
		case !at.Before(time.Now()):
			t.Errorf("a past date must come back as is: %v", at)
		}
	}
}

func FuzzRetryAfter(f *testing.F) {
	for _, s := range []string{"120", "0", "-1", "31536001", "9223372037", "Wed, 21 Oct 2026 07:28:00 GMT", "", "1e3", " 5"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, v string) {
		before := time.Now()
		at, ok := httpkit.RetryAfter(v)
		if !ok {
			return
		}
		if date, err := http.ParseTime(v); err == nil {
			if !at.Equal(date) {
				t.Fatalf("%q: %v is not the date", v, at)
			}
			return
		}
		if d := at.Sub(before); d <= 0 || d > 365*24*time.Hour+time.Minute {
			t.Fatalf("%q: %v from now", v, d)
		}
	})
}
