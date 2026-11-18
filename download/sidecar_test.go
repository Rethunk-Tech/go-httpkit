package download

import (
	"bytes"
	"os"
	"testing"
)

func TestAnOversizedSidecarDoesNotVouchForAPartial(t *testing.T) {
	t.Parallel()
	dest := newDest(t)
	if err := os.WriteFile(dest, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A complete sidecar in the first cap bytes, then spaces: a silent truncate would still resume.
	body := bytes.Repeat([]byte(" "), maxSidecar+1)
	copy(body, `{"url":"https://example/f"}`)
	if err := os.WriteFile(Sidecar(dest), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Partial(dest); got != 0 {
		t.Fatalf("Partial = %d", got)
	}
}

func TestPartialIsZeroWhenTheFileIsMissing(t *testing.T) {
	t.Parallel()
	dest := newDest(t)
	saveSidecar(dest, sidecar{URL: "https://example/f"})
	if got := Partial(dest); got != 0 {
		t.Fatalf("Partial = %d", got)
	}
}
