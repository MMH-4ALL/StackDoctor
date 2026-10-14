package detectors

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MMH-4ALL/StackDoctor/pkg/diagnostic"
)

type fakeTools map[string]string

func (f fakeTools) Probe(_ context.Context, name string) diagnostic.Tool {
	v, ok := f[name]
	return diagnostic.Tool{Name: name, Available: ok, Version: v, Problem: "not found on PATH"}
}

type fakePorts map[int]bool

func (f fakePorts) Listening(_ context.Context, port int) (bool, error) { return f[port], nil }

func fixtures(t *testing.T, files map[string]string) (*diagnostic.Context, string) {
	t.Helper()
	root := t.TempDir()
	for name, text := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	fs, _, err := diagnostic.OpenProject(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	return &diagnostic.Context{Files: fs, Tools: fakeTools{"node": "22.1.0", "npm": "10.8.0", "python3": "3.12.4", "go": "1.26.9", "rustc": "1.90.0", "cargo": "1.90.0"}, Ports: fakePorts{}}, root
}

func has(f []diagnostic.Finding, id string, severity diagnostic.Severity) bool {
	for _, item := range f {
		if item.ID == id && item.Severity == severity {
			return true
		}
	}
	return false
}

func TestNodeDiagnostics(t *testing.T) {
	c, _ := fixtures(t, map[string]string{"package.json": `{"engines":{"node":">=24"},"dependencies":{"vite":"^6.0.0"},"scripts":{"dev":"vite --port 4321"}}`, "package-lock.json": `{"lockfileVersion":3}`, "pnpm-lock.yaml": "lockfileVersion: 9.0"})
	f := Node{}.Detect(context.Background(), c)
	for _, tc := range []struct {
		id string
		s  diagnostic.Severity
	}{{"node.version.engines", diagnostic.Error}, {"node.dependencies.missing", diagnostic.Error}, {"node.locks.conflict", diagnostic.Warning}} {
		if !has(f, tc.id, tc.s) {
			t.Errorf("missing %s: %+v", tc.id, f)
		}
	}
	if got := inferredPorts(c); got[4321] == "" || len(got) != 1 {
		t.Fatalf("wrong port inference: %+v", got)
	}
}

func TestNodePackagesPresent(t *testing.T) {
	c, _ := fixtures(t, map[string]string{"package.json": `{"dependencies":{"@scope/pkg":"^1.0"},"engines":{"node":">=20 <24"}}`, "node_modules/@scope/pkg/package.json": "{}"})
	f := Node{}.Detect(context.Background(), c)
	if !has(f, "node.dependencies.present", diagnostic.Pass) || !has(f, "node.version.engines", diagnostic.Pass) {
		t.Fatalf("presence: %+v", f)
	}
}

func TestNodeMissingRuntime(t *testing.T) {
	c, _ := fixtures(t, map[string]string{"package.json": "{}"})
	c.Tools = fakeTools{}
	if !has(Node{}.Detect(context.Background(), c), "node.runtime.missing", diagnostic.Error) {
		t.Fatal("missing runtime not detected")
	}
}

func TestNodeInvalidAndPnP(t *testing.T) {
	for _, manifest := range []string{"null", "[]", `{"engines": {"node":42}}`, `{"secret":"do-not-leak"`} {
		c, _ := fixtures(t, map[string]string{"package.json": manifest})
		f := Node{}.Detect(context.Background(), c)
		if !has(f, "node.manifest.invalid", diagnostic.Error) {
			t.Fatal("bad manifest accepted")
		}
		b, _ := json.Marshal(f)
		if strings.Contains(string(b), "do-not-leak") {
			t.Fatal("manifest value leaked")
		}
	}
	c, _ := fixtures(t, map[string]string{"package.json": `{"dependencies":{"vite":"1"}}`, ".pnp.cjs": "throw Error('must-not-execute')"})
	f := Node{}.Detect(context.Background(), c)
	if !has(f, "node.dependencies.pnp", diagnostic.Info) || has(f, "node.dependencies.missing", diagnostic.Error) {
		t.Fatal("PnP incorrectly treated as absent packages")
	}
}

func TestPythonLocalDependenciesAndVersion(t *testing.T) {
	c, _ := fixtures(t, map[string]string{"pyproject.toml": "[project]\nrequires-python = '>=3.11,<3.13'\ndependencies = ['requests>=2.30', 'flask==3.1.0', 'platform-dep; sys_platform == \"win32\"']\n", ".venv/pyvenv.cfg": "version = 3.12.4\nhome = /private/do-not-leak\n", ".venv/lib/python3.12/site-packages/requests-2.29.0.dist-info/METADATA": "ignored"})
	f := Python{}.Detect(context.Background(), c)
	for _, tc := range []struct {
		id string
		s  diagnostic.Severity
	}{{"python.version.rule", diagnostic.Pass}, {"python.dependencies.missing", diagnostic.Error}, {"python.dependencies.version", diagnostic.Error}, {"python.dependencies.partial", diagnostic.Info}} {
		if !has(f, tc.id, tc.s) {
			t.Errorf("missing %s", tc.id)
		}
	}
	b, _ := json.Marshal(f)
	if strings.Contains(string(b), "do-not-leak") {
		t.Fatal("virtual env config leaked")
	}
}

func TestPythonExternalEnvironmentIsUnverified(t *testing.T) {
	c, _ := fixtures(t, map[string]string{"requirements.txt": "requests>=2.0\n-r ../outside.txt\nhttps://private:secret@example.com/pkg.whl"})
	f := Python{}.Detect(context.Background(), c)
	if !has(f, "python.venv.missing", diagnostic.Warning) {
		t.Fatal("absence not explained")
	}
	b, _ := json.Marshal(f)
	if strings.Contains(string(b), "private:secret") {
		t.Fatal("requirement URL leaked")
	}
}

func TestPythonWindowsLayout(t *testing.T) {
	c, _ := fixtures(t, map[string]string{"requirements.txt": "my-package~=1.2.0", ".venv/Lib/site-packages/my_package-1.2.3.dist-info/METADATA": "Name: my-package"})
	if !has(Python{}.Detect(context.Background(), c), "python.dependencies.present", diagnostic.Pass) {
		t.Fatal("Windows venv layout not detected")
	}
}

func TestPythonConstraints(t *testing.T) {
	for _, tc := range []struct {
		version, rule string
		want          bool
	}{{"1.2.3", "~=1.2.0", true}, {"1.3.0", "~=1.2.0", false}, {"3.12.4", ">=3.11,<3.13", true}, {"3.13.0", ">=3.11,<3.13", false}} {
		rule, ok := pythonConstraint(tc.rule)
		if !ok {
			t.Fatal(tc.rule)
		}
		match, known := compatible(tc.version, rule)
		if !known || match != tc.want {
			t.Errorf("%s %s", tc.version, tc.rule)
		}
	}
}

func TestGoDiagnostics(t *testing.T) {
	c, _ := fixtures(t, map[string]string{"go.mod": "module example.com/app\ngo 1.27\nrequire example.com/dep v1.0.0\nreplace example.com/local => ../outside\n"})
	f := Go{}.Detect(context.Background(), c)
	for _, tc := range []struct {
		id string
		s  diagnostic.Severity
	}{{"go.version.minimum", diagnostic.Error}, {"go.dependencies.sum", diagnostic.Warning}, {"go.replace.external", diagnostic.Warning}, {"go.dependencies.cache", diagnostic.Info}} {
		if !has(f, tc.id, tc.s) {
			t.Errorf("missing %s", tc.id)
		}
	}
}

func TestGoWorkspaceAndInvalidManifest(t *testing.T) {
	c, _ := fixtures(t, map[string]string{"go.work": "go 1.24\nuse (\n ./missing\n ../external\n)\n"})
	f := Go{}.Detect(context.Background(), c)
	if !has(f, "go.workspace.missing", diagnostic.Error) || !has(f, "go.workspace.external", diagnostic.Warning) {
		t.Fatalf("workspace: %+v", f)
	}
	c, _ = fixtures(t, map[string]string{"go.mod": "secret-value-bad-syntax"})
	f = Go{}.Detect(context.Background(), c)
	if !has(f, "go.manifest.invalid", diagnostic.Error) {
		t.Fatal("bad go.mod accepted")
	}
	b, _ := json.Marshal(f)
	if strings.Contains(string(b), "secret-value") {
		t.Fatal("parser error leaked")
	}
}

func TestRustDiagnostics(t *testing.T) {
	c, _ := fixtures(t, map[string]string{"Cargo.toml": "[package]\nname='app'\nversion='0.1.0'\nrust-version='1.95'\nedition='2024'\n[dependencies]\nlocal={path='missing'}\noutside={path='../outside'}\n", "rust-toolchain.toml": "[toolchain]\nchannel='nightly'\n"})
	f := Rust{}.Detect(context.Background(), c)
	for _, tc := range []struct {
		id string
		s  diagnostic.Severity
	}{{"rust.version.minimum", diagnostic.Error}, {"rust.dependencies.path.missing", diagnostic.Error}, {"rust.dependencies.path.external", diagnostic.Warning}, {"rust.toolchain.channel.rust-toolchain.toml", diagnostic.Info}} {
		if !has(f, tc.id, tc.s) {
			t.Errorf("missing %s: %+v", tc.id, f)
		}
	}
}

func TestEnvironmentSecretsNeverExport(t *testing.T) {
	c, root := fixtures(t, map[string]string{".env.example": "API_TOKEN=example-secret\nDATABASE_URL=template-secret\nPORT=3000", ".env": "API_TOKEN=actual-secret\nPORT=48123"})
	f := Env{}.Detect(context.Background(), c)
	if !has(f, "env.keys..env.example", diagnostic.Warning) {
		t.Fatal("missing env key not detected")
	}
	r, err := Builtins().Scan(context.Background(), diagnostic.Options{Path: root, Tools: fakeTools{}, PortProvider: fakePorts{}, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(r)
	for _, secret := range []string{"example-secret", "template-secret", "actual-secret"} {
		if strings.Contains(string(b), secret) {
			t.Fatalf("secret leaked: %s", secret)
		}
	}
	if p := inferredPorts(c); p[48123] == "" {
		t.Fatal("numeric port not inferred")
	}
}

func TestEnvironmentMissingFiles(t *testing.T) {
	c, _ := fixtures(t, map[string]string{".env.local.example": "TOKEN=hidden", ".env.sample": "KEY=hidden"})
	f := Env{}.Detect(context.Background(), c)
	if !has(f, "env.file..env.local.example", diagnostic.Warning) || !has(f, "env.file..env.sample", diagnostic.Warning) {
		t.Fatal("environment file names not inferred")
	}
	keys := EnvKeys([]byte("export GOOD=value\n# SECRET=hidden\ninvalid-key=hidden\nTOKEN=multiline\n'quoted line'\n"))
	if len(keys) != 2 || !keys["GOOD"] || !keys["TOKEN"] {
		t.Fatalf("keys: %+v", keys)
	}
}

func TestPortOverrideAndSkip(t *testing.T) {
	c, _ := fixtures(t, map[string]string{"package.json": `{"devDependencies":{"vite":"1"}}`})
	c.ExplicitPorts = []int{48123}
	c.Ports = fakePorts{48123: true}
	f := Ports{}.Detect(context.Background(), c)
	if !has(f, "ports.48123", diagnostic.Warning) || len(f) != 1 {
		t.Fatal("explicit port did not override guesses")
	}
	c.SkipPorts = true
	if !has(Ports{}.Detect(context.Background(), c), "ports.skipped", diagnostic.Info) {
		t.Fatal("no-ports ignored")
	}
}

func TestScanDetectsAllStacks(t *testing.T) {
	_, root := fixtures(t, map[string]string{"package.json": "{}", "requirements.txt": "requests", "go.mod": "module example.com/app\ngo 1.24\n", "Cargo.toml": "[workspace]\n"})
	r, err := Builtins().Scan(context.Background(), diagnostic.Options{Path: root, Tools: fakeTools{}, PortProvider: fakePorts{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Stacks) != 4 {
		t.Fatalf("stacks: %+v", r.Stacks)
	}
	ids := map[string]bool{}
	for _, f := range r.Findings {
		if ids[f.ID] {
			t.Fatalf("duplicate finding ID %s", f.ID)
		}
		ids[f.ID] = true
	}
}
