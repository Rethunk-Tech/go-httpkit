package download

import (
	"net/http"
	"testing"
)

func TestHashFromStripsSHA256PrefixAnyCase(t *testing.T) {
	h := make(http.Header)
	h.Set("X-Checksum-Sha256", "SHA256:deadbeef")
	if got, want := hashFrom(h), "sha256:deadbeef"; got != want {
		t.Fatalf("hashFrom SHA256:hex = %q, want %q", got, want)
	}
}
