# Security

## Supported versions

The latest tagged release. There are no long-lived release branches, so a fix ships in the next tag
rather than being backported.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting on this repository: <https://github.com/Rethunk-Tech/go-httpkit/security/advisories/new>, or the **Security** tab, then **Report a vulnerability**. The report stays private between you and us until a fix ships.

If private reporting is unavailable, email [oss@rethunk.tech](mailto:oss@rethunk.tech) with **[SECURITY]** in the subject.

Please do not open a public issue, pull request or discussion for a suspected vulnerability.

A useful report has the affected repository and version (or commit), steps to reproduce, what an attacker gets and under which conditions, and whether anyone else knows.

## What counts as a vulnerability

A vulnerability is a flaw that lets someone cross a boundary they should not: run code, read or change data they have no right to, escape a sandbox or path restriction, or leak a credential. A bug is anything else that misbehaves, such as a crash on your own input, wrong output or a slow path; open an ordinary issue for those. If you are unsure which one you have, report it privately and we will say.

## Coordinated disclosure

We handle the report inside the private advisory, so you see each status change and can comment on the fix. The advisory goes public when the fix ships, or 90 days after your report, whichever comes first. If a fix is close, either of us can ask for more time in the thread. You are credited in the advisory unless you would rather not be.

## Response timelines

- We acknowledge your report within 3 business days.
- We give you an assessment (accepted, needs more information or declined, with reasons) within 10 business days.
- We aim to ship a fix within 90 days of the report.

## What is in scope

- A response body: `DoCapped` and `download.File` hold at most the limit they are given, then fail with
  `safefs.ErrTooLarge`; `RangeReader` holds at most one chunk. A server announcing more than the limit is refused
  before any byte is written.
- A redirect: a credential must not follow one off its origin or from https to another scheme. `NoDowngrade` and
  `SameOrigin` refuse both (`TestNoDowngradeAndSameOrigin`, `TestFileRefusesAnHTTPSDowngrade`), and `WithRedirect`
  keeps them in force on a client that has its own policy (`TestWithRedirectChainsAndLeavesTheClientAlone`).
- A credential header: a redirect to another host also loses every credential-looking header (`NoDowngrade`,
  `StripCrossHost`) and everything `download.Options.Authorize` sets, since net/http strips only Authorization,
  Cookie and WWW-Authenticate. Error texts carry the URL without its query (`RedactURL`).
- A ranged read: `RangeReader` fails with `io.ErrUnexpectedEOF` on a 206 whose `Content-Range` or length is not the
  range asked for. A nil client has `DefaultTimeout`.
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

## Safe harbour

We will not pursue legal action against good-faith research that avoids privacy violations, data destruction and disruption of services, stops at a proof of concept, and keeps the details private until the advisory is published. Rethunk.Tech, LLC maintains this repository.
