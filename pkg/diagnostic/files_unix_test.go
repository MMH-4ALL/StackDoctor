//go:build !windows

package diagnostic

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
)

func TestFIFOIsRejectedWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	f, _, err := OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Read("pipe"); !errors.Is(err, ErrNotRegular) {
		t.Fatal("FIFO not rejected")
	}
}
