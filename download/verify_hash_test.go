package download

import (
	"net/http"
	"strings"
	"testing"
)

func TestHashFromStripsSHA256PrefixAnyCase(t *testing.T) {
	h := make(http.Header)
	h.Set("X-Checksum-Sha256", "SHA256:deadbeef")
	if got, want := hashFrom(h), "sha256:deadbeef"; got != want {
		t.Fatalf("hashFrom SHA256:hex = %q, want %q", got, want)
	}
}

func FuzzHashFrom(f *testing.F) {
	for _, s := range []string{"", "deadbeef", "sha256:deadbeef", "SHA256:DEADBEEF", " sha256:ab ", "sha256:"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, v string) {
		h := http.Header{"X-Checksum-Sha256": []string{v}}
		got := hashFrom(h)
		trim := strings.TrimSpace(v)
		if trim == "" {
			if got != "" {
				t.Fatalf("empty header: %q", got)
			}
			return
		}
		want := "sha256:" + strings.ToLower(strings.TrimPrefix(strings.ToLower(trim), "sha256:"))
		if got != want {
			t.Fatalf("%q → %q, want %q", v, got, want)
		}
	})
}
