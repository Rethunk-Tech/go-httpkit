# HUMANS.md

Run and use `go-httpkit`. Internals: [AGENTS.md](AGENTS.md).

## Quick start

```bash
go get github.com/Rethunk-Tech/go-httpkit
```

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Rethunk-Tech/go-httpkit"
	"github.com/Rethunk-Tech/go-httpkit/download"
)

func main() {
	// One identity for every client in the program, and a place to watch reachability.
	http.DefaultTransport = &httpkit.Transport{Base: http.DefaultTransport, UserAgent: "my-app/1.0"}

	ctx := context.Background()
	api := httpkit.WithRedirect(nil, httpkit.SameOrigin) // Keeps an API key on its own origin.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/v1/items", nil)
	if err != nil {
		panic(err)
	}
	body, err := httpkit.DoCapped(api, req, 1<<20, nil)
	var rate *httpkit.RateLimitError
	switch {
	case errors.Is(err, httpkit.ErrNotFound):
		fmt.Println("gone")
	case errors.As(err, &rate):
		fmt.Println("try again after", rate.Reset)
	case err != nil:
		fmt.Println("failed:", err)
	default:
		fmt.Println(len(body), "bytes")
	}

	// Resume a large download across restarts: at most 200 MiB, one shared speed cap.
	limiter := &download.Limiter{Rate: func() int64 { return 1 << 20 }}
	err = download.File(ctx, nil, "https://cdn.example.com/big.zip", "big.zip", download.Options{
		Limit:    200 << 20,
		Limiter:  limiter,
		Idle:     20 * time.Second,
		Progress: func(done, total int64) { fmt.Println(done, "of", total) },
	})
	if errors.Is(err, download.ErrLinkExpired) {
		fmt.Println("fetch a fresh link and call File again; the partial resumes")
	}
}
```

## Contracts

| Call | Contract |
| --- | --- |
| `DoCapped` | Returns the body of a 200 answer, at most `limit` bytes; any other status is a `*StatusError`, a longer body is `safefs.ErrTooLarge`. `check` sees every response before the status test. The body is always closed. A transport error comes back as `hc.Do` returned it. |
| `StatusError`, `RateLimitError` | `errors.Is` matches `ErrNotFound` for 404 and `ErrRateLimited` for 429 and for every `RateLimitError`. |
| `RetryAfter` | `ok` is false for empty, unparseable, `"0"`, negative or over 365 days of seconds. A past HTTP-date is returned as is. |
| `NoDowngrade`, `SameOrigin` | Stop after 10 hops. `NoDowngrade` refuses only https to another scheme; `SameOrigin` refuses another scheme or host than the first request's. |
| `NoFollow` | Returns the redirect answer instead of following it. |
| `WithRedirect` | Copies the client; runs the client's own policy first, then yours. The original is never modified. |
| `Transport` | Sets the User-Agent only on a request that has none, on a clone. `OnResult` sees every response and transport error and must not read the body. |
| `download.File` | `Limit` is required. Resumes with Range and If-Range, starts over on any 200, 416 or mismatched validator, and gives up with `ErrNoResume` after 4 attempts. 401, 403 and 410 are `ErrLinkExpired`; other unexpected statuses are `*httpkit.StatusError`. A cancel, stall or network error keeps the partial; a size or checksum mismatch removes it. |
| `download.Options.Idle` | Zero is 30 s, negative disables. A body `Read` blocked this long fails with `ErrIdle`, which matches `context.DeadlineExceeded`. |
| `download.Limiter` | One bucket shared by every holder; `Rate` is read on every chunk; nil or `<= 0` is unlimited. |
| `download.Sidecar`, `Partial`, `Remove` | The `.resume.json` file beside a partial, the bytes `File` can resume from, and removal of both. |
| `download.RangeReader` | An `io.ReaderAt` over ranged GETs; fails with `ErrRangeBudget` past `MaxRequests` (default 8) and with a `*httpkit.StatusError` when the server does not answer 206. Not safe for concurrent use. |

## Requirements

Go 1.27.3 or newer, plus `lefthook` and `gitleaks` on PATH if you want the git hooks.

```bash
lefthook install
```

That is per clone; git does not commit hooks. Pre-commit scans staged changes for secrets, and
pre-push runs `gate`.

## Verify

```bash
gate
```
