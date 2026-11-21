# Changelog

Notable changes to `go-httpkit`. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.2] - 2026-11-21

### Fixed

- `download.File` removes the `.resume.json` sidecar once a download is verified. It used to stay, so a later call with the same `dest` found a full-size file the sidecar vouched for and returned without a request, even when the remote file had changed.
- A FIFO, device or folder as a download `dest` or sidecar is refused instead of blocking the call; the sidecar is read with `safefs.ReadFileCapped`.
- The error for a malformed download URL no longer includes its credentials.
- A resumed body is bounded to its `Content-Range`; the unit is matched without regard to case, an empty 206 of a non-empty range is refused, and the `sha256:` checksum prefix is matched without regard to case.
- `Authorize` headers are kept across redirects that only add or drop a default port.

## [0.3.1] - 2026-11-12

### Fixed

- `StripCrossHost` (and `NoDowngrade`) keep credentials stripped for the rest of a redirect chain once a hop leaves the first request's origin, so A to B to A no longer hands them back to A.
- A credential header set under a non-canonical key no longer crosses hosts, and `StripCrossHost` drops credential headers on an https to http redirect.
- `SameOrigin` and `StripCrossHost` treat host case and an explicit default port as the same host.
- A server that never sends headers fails `download.File` with `ErrIdle` instead of hanging it, and a nil client no longer gets a 30-minute total timeout.
- A `Content-MD5` of a gzip-coded body no longer fails the decoded file, and one from a 206 is checked against the finished file.
- A short 206 with an unknown total is treated as the whole file, `RangeReader` reuses its tail chunk, refuses a file that changed between chunks, and follows an HTTPS-to-HTTP redirect.

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

[Unreleased]: https://github.com/Rethunk-Tech/go-httpkit/compare/v0.3.2...HEAD
[0.3.2]: https://github.com/Rethunk-Tech/go-httpkit/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/Rethunk-Tech/go-httpkit/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/Rethunk-Tech/go-httpkit/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/Rethunk-Tech/go-httpkit/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Rethunk-Tech/go-httpkit/releases/tag/v0.1.0
