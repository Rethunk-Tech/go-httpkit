<h1 align="center">go-httpkit</h1>

<div align="center">

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.27.3+-00ADD8?logo=go&logoColor=white)](go.mod)

</div>

---

HTTP client plumbing: capped requests, typed status and rate-limit errors, Retry-After, redirect policies, and resumable verified file downloads. Package `httpkit`, module `github.com/Rethunk-Tech/go-httpkit`.

A program that talks to several sites and downloads large files needs the same few things every time, and each is
easy to get subtly wrong: a body read with no size bound, a `Retry-After` that overflows, an API key that follows a
redirect to another host, a download that restarts from zero after a dropped connection. This module is the one
correct copy of each.

## Quick start

```bash
go get github.com/Rethunk-Tech/go-httpkit
```

A worked example and the contracts: [HUMANS.md](HUMANS.md).

## Features

- `DoCapped` reads an answer at most N bytes and turns every other status into a `*StatusError` that
  `errors.Is` matches against `ErrNotFound` and `ErrRateLimited`.
- `RetryAfter` reads both header forms and refuses values that would overflow a `Duration`.
- `NoDowngrade`, `SameOrigin` and `NoFollow` keep credentials where they belong; `WithRedirect` adds one to a
  client without losing the client's own.
- `Transport` sets a default User-Agent and reports every round trip.
- `download.File` resumes, verifies and size-limits a download, fails a stalled body with `ErrIdle`, and shares one
  bandwidth `Limiter` across downloads.
- `download.RangeReader` reads a remote file's parts through ranged GETs.

## Documentation

| Doc | Audience |
| --- | --- |
| [HUMANS.md](HUMANS.md) | Running and using the package |
| [AGENTS.md](AGENTS.md) | Internals, invariants, and why |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Process — commits, tests, docs |
| [SECURITY.md](SECURITY.md) | Vulnerability disclosure |
| [CHANGELOG.md](CHANGELOG.md) | Release notes |

## License

MIT — see [LICENSE](LICENSE). © 2026 Rethunk.Tech, LLC.
