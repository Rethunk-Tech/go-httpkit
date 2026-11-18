package download

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestVerifyReportsAMissingFileRatherThanAChecksum(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "gone.bin")
	hash := "sha256:" + strings.Repeat("0", 64)
	if err := verify(missing, sidecar{Hash: hash}, ""); err == nil {
		t.Fatal("a missing file passed against a recorded hash")
	} else if errors.Is(err, ErrChecksum) {
		t.Fatalf("a missing file looked like a checksum: %v", err)
	}
	if err := verify(missing, sidecar{}, hash); err == nil {
		t.Fatal("a missing file passed a digest")
	} else if errors.Is(err, ErrChecksum) {
		t.Fatalf("a missing file looked like a checksum: %v", err)
	}
}

func FuzzVerifyDigest(f *testing.F) {
	sum := sha256.Sum256([]byte("abc"))
	sum512 := sha512.Sum512([]byte("abc"))
	matchSHA := "sha256:" + hex.EncodeToString(sum[:])
	match512 := "sha512:" + hex.EncodeToString(sum512[:])
	const matchMD5 = "md5:900150983cd24fb0d6963f7d28e17f72"
	for _, s := range []string{matchSHA, match512, matchMD5, " SHA256:" + hex.EncodeToString(sum[:]) + " ", "", "blake3:00", "sha-256:00", "00", "sha256:00", "sha512:00", "md5:00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, digest string) {
		p := writeVerifyFile(t)
		err := verifyDigest(p, digest)
		switch d := strings.ToLower(strings.TrimSpace(digest)); {
		case d == "" || d == matchSHA || d == match512 || d == matchMD5:
			if err != nil {
				t.Fatalf("matching %q: %v", digest, err)
			}
		case strings.HasPrefix(d, "sha256:") || strings.HasPrefix(d, "sha512:") || strings.HasPrefix(d, "md5:"):
			if err == nil {
				t.Fatalf("mismatch passed: %q", digest)
			}
			if !errors.Is(err, ErrChecksum) {
				t.Fatalf("mismatch %q: %v", digest, err)
			}
			if _, statErr := os.Stat(p); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("mismatch left the file: %q", digest)
			}
		default:
			if err == nil {
				t.Fatalf("unknown form passed: %q", digest)
			}
		}
	})
}
