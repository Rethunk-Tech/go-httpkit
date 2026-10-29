package download

import (
	"encoding/json"
	"strings"

	safefs "github.com/Rethunk-Tech/go-safefs"
)

// SidecarSuffix ends the sidecar that records a partial download so a later File can resume it.
const SidecarSuffix = ".resume.json"

// maxSidecar bounds the read of a sidecar, which is input a crash or another program may have mangled.
const maxSidecar = 64 << 10

// sidecar is what File remembers about a partial download. The JSON tags are the on-disk format earlier writers
// produced, so they never change.
type sidecar struct {
	ExpectedSize int64  `json:"expectedSize"`
	ETag         string `json:"etag"`
	LastModified string `json:"lastModified,omitempty"`
	URL          string `json:"url"`
	Hash         string `json:"hash"`
	MD5          string `json:"md5,omitempty"`
}

// Sidecar is the sidecar path of the download at dest.
func Sidecar(dest string) string { return dest + SidecarSuffix }

// loadSidecar reads dest's sidecar; one that is missing, torn or unreadable is empty, so its partial counts as vouched for
// by nothing and is dropped.
func loadSidecar(dest string) sidecar {
	f, err := safefs.Open(Sidecar(dest))
	if err != nil {
		return sidecar{}
	}
	defer func() { _ = f.Close() }()
	b, err := safefs.ReadCapped(f, maxSidecar)
	if err != nil {
		return sidecar{}
	}
	var m sidecar
	if json.Unmarshal(b, &m) != nil {
		return sidecar{}
	}
	return m
}

// saveSidecar is best effort: a failed write costs a resume, never the download.
func saveSidecar(dest string, m sidecar) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	_ = safefs.WriteFileAtomic(Sidecar(dest), b, 0o600)
}

// ifRange is the validator a resume sends, or empty when none can (a weak ETag is no validator for If-Range).
func (m sidecar) ifRange() string {
	if m.ETag != "" && !strings.HasPrefix(m.ETag, "W/") {
		return m.ETag
	}
	return m.LastModified
}

// Partial is the number of bytes File can resume from at dest: its file when a sidecar names a URL, else 0.
func Partial(dest string) int64 {
	if loadSidecar(dest).URL == "" {
		return 0
	}
	st, err := safefs.Stat(dest)
	if err != nil {
		return 0
	}
	return st.Size()
}

// Remove deletes dest and its sidecar, ignoring absence.
func Remove(dest string) {
	_ = safefs.Remove(dest)
	_ = safefs.Remove(Sidecar(dest))
}
