# Changelog

Notable changes to `go-httpkit`. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.0] - 2026-11-08

### Added

- `NetReason` classifies a failed request as DNS, connection refused (including Windows WSAECONNREFUSED), timeout, or other.
- `download.Options.Digest`, a caller-supplied `algo:hex` checksum (`sha256`, `sha512` or `md5`) checked against the
  finished file; a mismatch wraps `ErrChecksum`.

## [0.2.0] - 2026-11-07

### Added

- `DoJSON`: a capped request that decodes a 200 JSON answer, with a 429 as `*RateLimitError`, other statuses as
  `*StatusError`, and `JSONOptions.Check` for a caller that reads a status itself.
- `StripCrossHost`, a redirect policy that drops credential headers on a redirect to another host; `RedactURL` and `RedactError` keep a query's signature or key out of error text; `DefaultTimeout` bounds a request made with a nil client.

### Fixed

- `download` treats a short or mismatched 206 as an error, and its errors drop the URL's query.
- Credential headers no longer follow a redirect to another host.

## [0.1.0] - 2026-10-29

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

[Unreleased]: https://github.com/Rethunk-Tech/go-httpkit/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/Rethunk-Tech/go-httpkit/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/Rethunk-Tech/go-httpkit/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Rethunk-Tech/go-httpkit/releases/tag/v0.1.0
