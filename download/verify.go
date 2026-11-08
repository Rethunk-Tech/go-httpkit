package download

import (
	"crypto/md5" // #nosec G501 -- Content-MD5 is MD5 by RFC 1864; Options.Digest allows md5:hex.
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"strings"

	safefs "github.com/Rethunk-Tech/go-safefs"
)

// hashFrom is the sidecar form of the X-Checksum-Sha256 header, or empty when the server sent none.
func hashFrom(h http.Header) string {
	if v := strings.TrimSpace(h.Get("X-Checksum-Sha256")); v != "" {
		return "sha256:" + strings.ToLower(strings.TrimPrefix(v, "sha256:"))
	}
	return ""
}

// verify checks the finished file against Options.Digest and every digest the server announced. A mismatch
// removes the file: it is corrupt, not interrupted.
func verify(dest string, meta sidecar, digest string) error {
	if err := verifyDigest(dest, digest); err != nil {
		return err
	}
	if meta.Hash == "" && meta.MD5 == "" {
		return nil
	}
	var wantMD5 []byte
	if meta.MD5 != "" {
		b, err := base64.StdEncoding.DecodeString(meta.MD5)
		if err != nil {
			Remove(dest)
			return fmt.Errorf("%w: Content-MD5 is not base64: %w", ErrChecksum, err)
		}
		wantMD5 = b
	}
	f, err := safefs.Open(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	sha := sha256.New()
	sum := md5.New() // #nosec G401 -- Content-MD5 is MD5 by RFC 1864.
	if _, err := io.Copy(io.MultiWriter(sha, sum), f); err != nil {
		return fmt.Errorf("read the download to verify it: %w", err)
	}
	if kind, want, _ := strings.Cut(meta.Hash, ":"); kind == "sha256" && !strings.EqualFold(want, hex.EncodeToString(sha.Sum(nil))) {
		_ = f.Close()
		Remove(dest)
		return fmt.Errorf("%w: sha256", ErrChecksum)
	}
	if meta.MD5 != "" && (len(wantMD5) != md5.Size || string(sum.Sum(nil)) != string(wantMD5)) {
		_ = f.Close()
		Remove(dest)
		return fmt.Errorf("%w: Content-MD5", ErrChecksum)
	}
	return nil
}

// verifyDigest checks path against a "sha256:<hex>", "sha512:<hex>" or "md5:<hex>" digest. An empty digest
// skips the check; any other form is refused so it cannot read as a passed check.
func verifyDigest(path, digest string) error {
	digest = strings.ToLower(strings.TrimSpace(digest))
	if want, ok := strings.CutPrefix(digest, "sha512:"); ok {
		return compareHash(path, sha512.New(), want, "sha512")
	}
	if want, ok := strings.CutPrefix(digest, "md5:"); ok {
		return compareHash(path, md5.New(), want, "md5") // #nosec G401 -- Options.Digest allows md5:hex.
	}
	if want, ok := strings.CutPrefix(digest, "sha256:"); ok {
		return compareHash(path, sha256.New(), want, "sha256")
	}
	if digest == "" {
		return nil
	}
	return fmt.Errorf("checksum %q is not a form this package can verify", digest)
}

func compareHash(path string, h hash.Hash, want, name string) error {
	f, err := safefs.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("read the download to verify it: %w", err)
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		_ = f.Close()
		Remove(path)
		return fmt.Errorf("%w: %s", ErrChecksum, name)
	}
	return nil
}
