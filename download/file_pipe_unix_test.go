//go:build unix

package download

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestFileDoesNotHangOnANamedPipeSidecar(t *testing.T) {
	t.Parallel()
	dest := filepath.Join(t.TempDir(), "f")
	if err := syscall.Mkfifo(Sidecar(dest), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := bodyServer(t, resumeBody, nil)
	done := make(chan error, 1)
	go func() { done <- fetch(t, srv, dest, Options{}) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("File hung opening a named-pipe sidecar")
	}
}

func TestFileDoesNotHangOnANamedPipeDest(t *testing.T) {
	t.Parallel()
	dest := filepath.Join(t.TempDir(), "f")
	if err := syscall.Mkfifo(dest, 0o600); err != nil {
		t.Fatal(err)
	}
	srv := bodyServer(t, resumeBody, nil)
	done := make(chan error, 1)
	go func() { done <- fetch(t, srv, dest, Options{}) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("File wrote through a named-pipe dest")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("File hung opening a named-pipe dest")
	}
}

func TestFileDoesNotWriteIntoANamedPipe(t *testing.T) {
	t.Parallel()
	dest := filepath.Join(t.TempDir(), "f")
	if err := syscall.Mkfifo(dest, 0o600); err != nil {
		t.Fatal(err)
	}
	leaked := make(chan []byte, 1)
	go func() {
		f, err := os.OpenFile(dest, os.O_RDONLY, 0) //nolint:gosec // G304: dest is the FIFO this test created.
		if err != nil {
			leaked <- nil
			return
		}
		defer func() { _ = f.Close() }()
		b, _ := io.ReadAll(f)
		leaked <- b
	}()
	srv := bodyServer(t, resumeBody, nil)
	_ = fetch(t, srv, dest, Options{})
	select {
	case got := <-leaked:
		if string(got) == resumeBody {
			t.Fatalf("the download was written into the named pipe: %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the pipe reader")
	}
}
