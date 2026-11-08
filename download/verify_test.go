package download

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeVerifyFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.bin")
	if err := os.WriteFile(p, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVerifyDigest(t *testing.T) {
	t.Parallel()
	sum := sha256.Sum256([]byte("abc"))
	sha2 := "sha256:" + hex.EncodeToString(sum[:])
	p := writeVerifyFile(t)
	meta := sidecar{}
	if err := verify(p, meta, sha2); err != nil {
		t.Errorf("matching digest: %v", err)
	}
	sum512 := sha512.Sum512([]byte("abc"))
	if err := verify(p, meta, "sha512:"+hex.EncodeToString(sum512[:])); err != nil {
		t.Errorf("matching sha512: %v", err)
	}
	if err := verify(p, meta, "md5:900150983cd24fb0d6963f7d28e17f72"); err != nil {
		t.Errorf("matching md5: %v", err)
	}
	if err := verify(p, meta, " SHA256:"+hex.EncodeToString(sum[:])+" "); err != nil {
		t.Errorf("trimmed matching digest: %v", err)
	}
	for _, digest := range []string{"blake3:00", "sha-256:00", "00"} {
		if err := verify(p, meta, digest); err == nil {
			t.Errorf("digest %q, which this build cannot check, passed", digest)
		}
	}
	if err := verify(p, meta, ""); err != nil {
		t.Errorf("no digest: %v", err)
	}
	wrong := writeVerifyFile(t)
	if err := verify(wrong, meta, "sha256:00"); err == nil || !errors.Is(err, ErrChecksum) {
		t.Errorf("a wrong digest: %v", err)
	}
	if _, err := os.Stat(wrong); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a wrong digest left the file: %v", err)
	}
	if err := verify(writeVerifyFile(t), meta, "sha512:00"); err == nil || !errors.Is(err, ErrChecksum) {
		t.Error("a wrong sha512 passed")
	}
	if err := verify(writeVerifyFile(t), meta, "md5:00"); err == nil || !errors.Is(err, ErrChecksum) {
		t.Error("a wrong md5 passed")
	}
}
