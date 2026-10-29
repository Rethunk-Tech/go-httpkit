package download

import (
	"crypto/md5" // #nosec G501 -- Content-MD5 is MD5 by RFC 1864.
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
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

// verify checks the finished file against every digest the server announced. A mismatch removes the file: it is
// corrupt, not interrupted.
func verify(dest string, meta sidecar) error {
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
