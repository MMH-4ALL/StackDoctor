package diagnostic

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

const MaxFileBytes = 2 << 20
const MaxDirectoryEntries = 4096

var (
	ErrUnsafePath     = errors.New("path is outside the project or is a symbolic link")
	ErrTooLarge       = errors.New("project file exceeds the 2 MiB read limit")
	ErrNotRegular     = errors.New("project file is not a regular file")
	ErrTooManyEntries = errors.New("directory exceeds the entry limit")
)

// Files gives detectors bounded, project-scoped access. No recursive walk is offered.
type Files interface {
	Read(string) ([]byte, error)
	Stat(string) (os.FileInfo, error)
	List(string) ([]string, error)
}

type ProjectFiles struct{ root *os.Root }

func OpenProject(path string) (*ProjectFiles, string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", errors.New("cannot resolve the project directory")
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, "", errors.New("project directory does not exist or is inaccessible")
	}
	r, err := os.OpenRoot(abs)
	if err != nil {
		return nil, "", errors.New("cannot open the project directory")
	}
	return &ProjectFiles{root: r}, abs, nil
}

func (f *ProjectFiles) Close() error { return f.root.Close() }

func local(name string) bool {
	return name == "." || (filepath.IsLocal(name) && filepath.VolumeName(name) == "")
}

func (f *ProjectFiles) Stat(name string) (os.FileInfo, error) {
	if !local(name) {
		return nil, ErrUnsafePath
	}
	i, err := f.root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if i.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnsafePath
	}
	return i, nil
}

func (f *ProjectFiles) Read(name string) ([]byte, error) {
	i, err := f.Stat(name)
	if err != nil {
		return nil, err
	}
	if !i.Mode().IsRegular() {
		return nil, ErrNotRegular
	}
	if i.Size() > MaxFileBytes {
		return nil, ErrTooLarge
	}
	file, err := f.root.OpenFile(name, os.O_RDONLY|nonBlock, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	i, err = file.Stat()
	if err != nil {
		return nil, err
	}
	if !i.Mode().IsRegular() {
		return nil, ErrNotRegular
	}
	b, err := io.ReadAll(io.LimitReader(file, MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxFileBytes {
		return nil, ErrTooLarge
	}
	return b, nil
}

func (f *ProjectFiles) List(name string) ([]string, error) {
	i, err := f.Stat(name)
	if err != nil {
		return nil, err
	}
	if !i.IsDir() {
		return nil, ErrNotRegular
	}
	d, err := f.root.OpenFile(name, os.O_RDONLY|nonBlock, 0)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	names, err := d.Readdirnames(MaxDirectoryEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(names) > MaxDirectoryEntries {
		return nil, ErrTooManyEntries
	}
	return names, nil
}

func Exists(f Files, name string) bool { _, err := f.Stat(name); return err == nil }
func IsDir(f Files, name string) bool  { i, err := f.Stat(name); return err == nil && i.IsDir() }
