package diagnostic

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectFilesBoundaries(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("must-not-read"), 0600); err != nil {
		t.Fatal(err)
	}
	f, _, err := OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if b, err := f.Read("manifest"); err != nil || string(b) != "ok" {
		t.Fatalf("read: %q %v", b, err)
	}
	for _, path := range []string{"../secret", filepath.Join(outside, "secret")} {
		if _, err := f.Read(path); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("unsafe path accepted: %v", err)
		}
	}
	if _, err := f.Read("."); !errors.Is(err, ErrNotRegular) {
		t.Errorf("directory read: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "link")); err == nil {
		if _, err := f.Read("link"); err == nil {
			t.Fatal("external symlink accepted")
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked-dir")); err == nil {
		if _, err := f.Read("linked-dir/secret"); err == nil {
			t.Fatal("intermediate external symlink accepted")
		}
	}
}

func TestReadLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "large"), []byte(strings.Repeat("x", MaxFileBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	f, _, err := OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Read("large"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("limit: %v", err)
	}
}

func TestTextRemovesControls(t *testing.T) {
	for _, input := range []string{"a\x1b[2Jb", "a\r\nb", "a\u202eb"} {
		out := Text(input)
		if strings.ContainsAny(out, "\x1b\r\n\u202e") {
			t.Fatal("control survives")
		}
	}
	if len(Text(strings.Repeat("a", 2000))) > 1610 {
		t.Fatal("text not bounded")
	}
}
