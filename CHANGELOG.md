# Changelog

Notable changes to `go-httpkit`. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `DoCapped`, a size-capped request whose non-200 answers are a typed `*StatusError` matching `ErrNotFound` and
  `ErrRateLimited`.
- `RateLimitError` and `RetryAfter`, which reads both Retry-After forms and refuses values that would overflow.
- Redirect policies `NoDowngrade`, `SameOrigin` and `NoFollow`, and `WithRedirect` to add one to a client without
  losing the client's own.
- `Transport`, which sets a default User-Agent and reports every round trip.
- `download.File`, a resumable, verified file download: it continues a partial with Range and If-Range, checks
  announced SHA-256 and Content-MD5 digests, enforces a size limit, and fails a stalled body with `ErrIdle`.
- `download.Limiter`, one bandwidth cap shared by every download that holds it, and `download.Sidecar`,
  `Partial` and `Remove` for the files a paused download leaves behind.
- `download.RangeReader`, an `io.ReaderAt` over a remote file's ranged GETs with a request budget.

[Unreleased]: https://github.com/Rethunk-Tech/go-httpkit/commits/main
