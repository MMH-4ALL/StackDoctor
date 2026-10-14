package detectors

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MMH-4ALL/StackDoctor/pkg/diagnostic"
)

func TestUnsafeManifestProducesExplicitError(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "package.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "package.json"), filepath.Join(root, "package.json")); err != nil {
		t.Skip("symlink creation unavailable")
	}
	r, err := Builtins().Scan(context.Background(), diagnostic.Options{Path: root, Tools: fakeTools{}, PortProvider: fakePorts{}})
	if err != nil {
		t.Fatal(err)
	}
	if !has(r.Findings, "project.marker.package.json", diagnostic.Error) {
		t.Fatal("unsafe marker silently ignored")
	}
}

func TestRustInheritedMetadataIsAccepted(t *testing.T) {
	c, _ := fixtures(t, map[string]string{"Cargo.toml": "[package]\nname='member'\nversion.workspace=true\nrust-version.workspace=true\nedition.workspace=true\n"})
	f := Rust{}.Detect(context.Background(), c)
	if has(f, "rust.manifest.invalid", diagnostic.Error) || !has(f, "rust.version.inherited", diagnostic.Info) {
		t.Fatalf("valid inherited metadata rejected: %+v", f)
	}
}

func TestPythonProjectInterpreterRemainsUnverified(t *testing.T) {
	c, _ := fixtures(t, map[string]string{"requirements.txt": "requests", ".venv/pyvenv.cfg": "version = 3.12.4"})
	c.Tools = fakeTools{}
	f := Python{}.Detect(context.Background(), c)
	if has(f, "python.runtime.missing", diagnostic.Error) || !has(f, "python.runtime.local", diagnostic.Warning) {
		t.Fatal("unexecuted local environment falsely declared missing")
	}
}

func TestRepeatedGoReplacementsHaveUniqueFindingIDs(t *testing.T) {
	c, _ := fixtures(t, map[string]string{"go.mod": "module example.com/app\ngo 1.24\nreplace example.com/a => ../a\nreplace example.com/b => ../b\nreplace example.com/c => ./missing-c\nreplace example.com/d => ./missing-d\n"})
	ids := map[string]bool{}
	for _, f := range (Go{}).Detect(context.Background(), c) {
		if ids[f.ID] {
			t.Fatalf("duplicate %s", f.ID)
		}
		ids[f.ID] = true
	}
}
