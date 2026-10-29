// Package download fetches large files over flaky links: a resumable, verified, throttled File, and a RangeReader
// that reads a remote file's parts without downloading all of it.
//
// A partial file and its sidecar (Sidecar) are a resumable pair: File continues it with Range and If-Range when the
// server and the recorded validators allow, and starts over otherwise. Every failure that cannot corrupt the file
// keeps the pair; a size or checksum mismatch removes it.
package download
