// Package httpkit is the HTTP client plumbing a program needs once it talks to more than one site: a size-capped
// request, typed status and rate-limit errors, one Retry-After reader, redirect policies that keep credentials with
// their origin, and a Transport that sets a default User-Agent and reports every round trip.
//
// Resumable file downloads live in the download subpackage.
package httpkit
