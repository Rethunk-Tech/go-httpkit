# AGENTS.md

Package `httpkit` is the HTTP client plumbing a program needs once it talks to more than one site: a capped request,
typed status and rate-limit errors, one Retry-After reader, redirect policies and a Transport. Package `download`
is the resumable, verified, throttled file download built on it. The module imports no consumer package, and its
only non-stdlib dependency is go-safefs.

The module carries no product wording: error text is plain English for a developer, and a caller that shows a user
something maps the typed errors itself. A host's own rate-limit answer and credentials reach `download.File` only
through `Options.Check` and `Options.Authorize`, so the module knows no host.

## Gate

`gate` is the command. Each override in `.gate.toml` is there because a runner
cannot resolve that check from PATH. No code is OS-specific; the `windows-latest` job
runs the same tests, which is where a rename or remove that Windows refuses shows up.

## Shape

- `StatusError` carries no Retry-After. The readers that need one hold the response and call `RetryAfter`
  themselves, so the error stays a status and nothing else.
- `RetryAfter` returns a past HTTP-date as is. What a reset not after now means is the caller's decision.
- `WithRedirect` chains the client's own `CheckRedirect` before the new policy and never yields to it, so a token
  guard cannot be switched off by passing a client that has a policy. There is no `HTTPSOnly`: for an https first
  hop `NoDowngrade` decides every redirect identically.
- `Transport.RoundTrip` clones the request before setting the User-Agent, because a RoundTripper must not modify
  the caller's request. `OnResult` receives the transport's own error, not the `*url.Error` that `http.Client` wraps
  it in, and must not read `resp.Body`.
- `download.File` writes the sidecar before the first body byte, so a crash mid-body leaves a resumable pair. A
  network error, stall or cancel keeps the pair; a size or digest mismatch removes it, because the bytes are wrong
  rather than interrupted.
- The sidecar's six JSON tags are an on-disk format that earlier writers produced, and `testdata/sidecar.json`
  holds the bytes one of them wrote. A sidecar that is missing, torn or over 64 KiB reads as empty, so its partial is
  vouched for by nothing and is dropped.
- Any 200 starts over and resets every recorded validator, size and digest: they describe another body. A 206 is
  accepted only when `Content-Range` is a real interval that starts at the offset and whose total matches the
  recorded size.
- The idle limit is a `context.WithCancelCause` on the request, armed only while a body `Read` is blocked. After
  the copy, `File` reads the cause and reports `ErrIdle` whatever the transport returned, because over HTTP/2 the
  body `Read` returns a bare `context.Canceled`. `ErrIdle` wraps `context.DeadlineExceeded` so a caller that treats
  deadlines as retryable retries a stall.
- `Limiter` is read on every chunk and shared by every holder; its burst is a quarter second of the rate, at least
  16 KiB.
- Timing tests run in a `testing/synctest` bubble over a `net.Pipe` transport (`download/helpers_test.go`):
  httptest's loopback listener is network IO the bubble does not count as durably blocked, so such a test hangs.
  HTTP/2 needs TLS over a real listener and cannot run in a bubble, so its idle test runs in real time.

## Files

| File | Holds |
| --- | --- |
| `doc.go` | Package documentation. |
| `capped.go` | `DoCapped`, `StatusError`, `ErrNotFound`, `ErrRateLimited`. |
| `ratelimit.go` | `RateLimitError`, `RetryAfter`. |
| `redirect.go` | `NoDowngrade`, `SameOrigin`, `NoFollow`, `WithRedirect`. |
| `transport.go` | `Transport`. |
| `download/download.go` | Sentinels, `Options`, `Progress`, the restart and idle constants. |
| `download/file.go` | `File`, divided into `prepare`, `accept` and `write`, and the idle-limit body wrapper. |
| `download/sidecar.go` | The sidecar format, `Sidecar`, `Partial`, `Remove`. |
| `download/contentrange.go` | The `Content-Range` parser. |
| `download/verify.go` | SHA-256 and Content-MD5 verification. |
| `download/limiter.go` | `Limiter`, the shared token bucket. |
| `download/rangereader.go` | `RangeReader`, `ErrRangeBudget`. |
| `download/doc.go` | Package documentation. |
