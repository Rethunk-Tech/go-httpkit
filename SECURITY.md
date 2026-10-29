# Security

## Supported versions

The latest tagged release. There are no long-lived release branches, so a fix ships in the next tag
rather than being backported.

## Reporting a vulnerability

Use GitHub's **private vulnerability reporting** on this repository (Security → Report a
vulnerability). It stays private until a fix ships.

Please do not open a public issue for a suspected vulnerability first.

## What is in scope

- A response body: `DoCapped` and `download.File` hold at most the limit they are given, then fail with
  `safefs.ErrTooLarge`; `RangeReader` holds at most one chunk. A server announcing more than the limit is refused
  before any byte is written.
- A redirect: a credential must not follow one off its origin or from https to another scheme. `NoDowngrade` and
  `SameOrigin` refuse both (`TestNoDowngradeAndSameOrigin`, `TestFileRefusesAnHTTPSDowngrade`), and `WithRedirect`
  keeps them in force on a client that has its own policy (`TestWithRedirectChainsAndLeavesTheClientAlone`).
- Response headers: `RetryAfter` never overflows a `Duration` (`FuzzRetryAfter`); the `Content-Range` parser accepts
  only a real interval (`FuzzContentRange`); neither panics.
- A sidecar file: `download.File` treats arbitrary sidecar bytes as an untrusted hint, never panics on them and
  never leaves a file over its limit (`FuzzSidecar`). It is read through a 64 KiB cap.
- A body that stalls: `download.File` fails it with `ErrIdle` after the idle limit instead of waiting forever.

## What is out of scope

- The URL, destination path and credentials the caller passes: the module does not decide which hosts are allowed.
  `download.File` fetches any URL it is given and refuses only an https-to-other redirect.
- An answer that is wrong but within the limit: `download.File` verifies only the digests the server announced, so a
  server that lies about its checksum is out of scope; the caller verifies against a digest it trusts.
- A rate-limit or credential policy specific to one host: it belongs in `Options.Check` and `Options.Authorize`.
