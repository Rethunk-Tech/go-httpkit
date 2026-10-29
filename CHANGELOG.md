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

[Unreleased]: https://github.com/Rethunk-Tech/go-httpkit/commits/main
