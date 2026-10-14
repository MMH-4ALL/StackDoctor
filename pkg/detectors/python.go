package detectors

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/MMH-4ALL/StackDoctor/pkg/diagnostic"
	"github.com/pelletier/go-toml/v2"
)

type Python struct{}

func (Python) ID() string { return "python" }

type pyProject struct {
	Project struct {
		RequiresPython string   `toml:"requires-python"`
		Dependencies   []string `toml:"dependencies"`
	} `toml:"project"`
	BuildSystem struct {
		Requires []string `toml:"requires"`
		Backend  string   `toml:"build-backend"`
	} `toml:"build-system"`
	Tool struct {
		Poetry struct {
			Dependencies map[string]any `toml:"dependencies"`
		} `toml:"poetry"`
	} `toml:"tool"`
}

var pyRequirement = regexp.MustCompile(`^([a-zA-Z0-9][a-zA-Z0-9_.-]*)(?:\[[a-zA-Z0-9_,.-]+\])?\s*([<>=!~0-9.*, ]*)$`)
var pyVersionDir = regexp.MustCompile(`^python[0-9]+\.[0-9]+$`)

func normalizePy(s string) string {
	return strings.ToLower(strings.NewReplacer("_", "-", ".", "-").Replace(s))
}

func pythonConstraint(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "~=") {
		version := strings.TrimSpace(strings.TrimPrefix(s, "~="))
		if !numericVersion.MatchString(version) {
			return "", false
		}
		parts := strings.Split(version, ".")
		var major, minor int
		if _, err := fmt.Sscanf(version, "%d.%d", &major, &minor); err != nil {
			return "", false
		}
		upper := fmt.Sprintf("%d.0", major+1)
		if len(parts) >= 3 {
			upper = fmt.Sprintf("%d.%d", major, minor+1)
		}
		return ">=" + version + ", <" + upper, true
	}
	if strings.Contains(s, "===") || strings.Contains(s, "~") || strings.Contains(s, "^") {
		return "", false
	}
	return s, true
}

func (Python) Detect(ctx context.Context, c *diagnostic.Context) []diagnostic.Finding {
	found := false
	for _, file := range []string{"pyproject.toml", "requirements.txt", "Pipfile", "setup.py", "setup.cfg"} {
		if diagnostic.Exists(c.Files, file) {
			found = true
		}
	}
	if !found {
		return nil
	}
	f := []diagnostic.Finding{}
	venv := ""
	for _, name := range []string{".venv", "venv"} {
		if diagnostic.IsDir(c.Files, name) {
			venv = name
			break
		}
	}
	t := c.Tools.Probe(ctx, "python3")
	if !t.Available {
		t = c.Tools.Probe(ctx, "python")
	}
	if !t.Available {
		t = c.Tools.Probe(ctx, "py")
	}
	if t.Available && t.Version != "" {
		f = append(f, finding("python.runtime.available", diagnostic.Runtime, diagnostic.Pass, "Python "+t.Version+" available", "An isolated version-only probe succeeded. No project imports were performed.", ""))
	} else if t.Available {
		f = append(f, finding("python.runtime.unknown", diagnostic.Runtime, diagnostic.Warning, "Python version unverified", "The interpreter was found but its version could not be determined.", ""))
	} else if venv != "" {
		f = append(f, finding("python.runtime.local", diagnostic.Runtime, diagnostic.Warning, "Local Python runtime is unverified", "A project virtual environment exists, but its interpreter is not executed. No trusted external Python probe succeeded.", venv))
	} else {
		f = append(f, finding("python.runtime.missing", diagnostic.Runtime, diagnostic.Error, "Python is unavailable", "No usable python3, python, or Windows py interpreter was found on PATH.", "", repair("Install Python 3 from python.org and reopen your terminal.", "")))
	}
	deps := map[string]string{}
	skipped := 0
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") {
			return
		}
		if i := strings.Index(raw, " #"); i >= 0 {
			raw = strings.TrimSpace(raw[:i])
		}
		m := pyRequirement.FindStringSubmatch(raw)
		if len(m) == 0 {
			skipped++
			return
		}
		deps[normalizePy(m[1])] = strings.TrimSpace(m[2])
	}
	rule, ruleFile := "", ""
	if diagnostic.Exists(c.Files, "pyproject.toml") {
		b, err := c.Files.Read("pyproject.toml")
		var p pyProject
		if err != nil {
			f = append(f, unreadable("python.manifest.unreadable", "pyproject.toml"))
		} else if toml.Unmarshal(b, &p) != nil {
			f = append(f, finding("python.manifest.invalid", diagnostic.Configuration, diagnostic.Error, "pyproject.toml cannot be parsed", "Correct the TOML syntax and the types of project fields. Parser errors and values are not echoed.", "pyproject.toml"))
		} else {
			f = append(f, finding("python.manifest.valid", diagnostic.Configuration, diagnostic.Pass, "pyproject.toml parsed", "Python project detected.", "pyproject.toml"))
			rule, ruleFile = p.Project.RequiresPython, "pyproject.toml"
			for _, req := range p.Project.Dependencies {
				add(req)
			}
			for name, value := range p.Tool.Poetry.Dependencies {
				if name == "python" {
					if rule == "" {
						if v, ok := value.(string); ok {
							rule = v
						}
					}
					continue
				}
				if v, ok := value.(string); ok && v == "*" {
					add(name)
				} else {
					skipped++
				}
			}
			if p.BuildSystem.Backend != "" && len(p.BuildSystem.Requires) == 0 {
				f = append(f, finding("python.build.requires", diagnostic.Configuration, diagnostic.Warning, "Build backend has no declared requirements", "Check build-system.requires for the packages needed to load the build backend.", "pyproject.toml"))
			}
		}
	}
	if diagnostic.Exists(c.Files, "requirements.txt") {
		if b, err := c.Files.Read("requirements.txt"); err != nil {
			f = append(f, unreadable("python.requirements.unreadable", "requirements.txt"))
		} else {
			for _, line := range strings.Split(string(b), "\n") {
				add(line)
			}
		}
	}
	if diagnostic.Exists(c.Files, "Pipfile") {
		b, err := c.Files.Read("Pipfile")
		var p struct {
			Packages map[string]any `toml:"packages"`
			Requires struct {
				PythonVersion string `toml:"python_version"`
			} `toml:"requires"`
		}
		if err != nil {
			f = append(f, unreadable("python.pipfile.unreadable", "Pipfile"))
		} else if toml.Unmarshal(b, &p) != nil {
			f = append(f, finding("python.pipfile.invalid", diagnostic.Configuration, diagnostic.Error, "Pipfile cannot be parsed", "Correct the TOML syntax. Values are not echoed.", "Pipfile"))
		} else {
			for name, v := range p.Packages {
				if s, ok := v.(string); ok {
					if s == "*" {
						add(name)
					} else {
						add(name + s)
					}
				} else {
					skipped++
				}
			}
			if rule == "" && p.Requires.PythonVersion != "" {
				rule = p.Requires.PythonVersion
				ruleFile = "Pipfile"
			}
		}
	}
	selectedVersion := t.Version
	if venv != "" {
		if b, err := c.Files.Read(filepath.Join(venv, "pyvenv.cfg")); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 && strings.TrimSpace(parts[0]) == "version" && numericVersion.MatchString(strings.TrimSpace(parts[1])) {
					selectedVersion = strings.TrimSpace(parts[1])
				}
			}
		}
		f = append(f, finding("python.venv.present", diagnostic.Dependencies, diagnostic.Pass, "Project virtual environment found", "Found "+venv+". Only local directory metadata is inspected; its interpreter is not executed.", venv))
	}
	if rule != "" && selectedVersion != "" {
		converted, ok := pythonConstraint(rule)
		if !ok {
			f = append(f, finding("python.version.rule", diagnostic.Runtime, diagnostic.Warning, "Python version rule needs review", "This version rule is outside the supported PEP 440 subset. Inspect it locally.", ruleFile))
		} else {
			f = append(f, versionFinding("python.version.rule", "Python", selectedVersion, converted, ruleFile))
		}
	}
	if diagnostic.Exists(c.Files, ".python-version") {
		b, err := c.Files.Read(".python-version")
		if err != nil {
			f = append(f, unreadable("python.version.pin", ".python-version"))
		} else if pin := strings.TrimSpace(string(b)); numericVersion.MatchString(pin) && selectedVersion != "" {
			f = append(f, versionFinding("python.version.pin", "Python", selectedVersion, pin, ".python-version"))
		} else {
			f = append(f, finding("python.version.pin", diagnostic.Runtime, diagnostic.Info, "Python version-manager pin needs review", "Only a single numeric interpreter version is supported offline.", ".python-version"))
		}
	}
	if len(deps) > 0 {
		if venv == "" {
			f = append(f, finding("python.venv.missing", diagnostic.Dependencies, diagnostic.Warning, "No local Python environment to inspect", "Declared packages exist, but .venv or venv was not found. Active, global, Conda, and externally managed environments are not inspected.", "", repair("Create a project virtual environment with your chosen Python interpreter.", "python -m venv .venv"), repair("Activate the environment, then review and install the declared dependencies.", "")))
		} else {
			installed, readOK := pythonPackages(c, venv)
			if !readOK {
				f = append(f, finding("python.dependencies.unverified", diagnostic.Dependencies, diagnostic.Warning, "Python package metadata unavailable", "No readable site-packages directory was found in the local environment. Recreate it or inspect its layout.", venv))
			} else {
				missing := []string{}
				mismatched := []string{}
				for name, rule := range deps {
					v, ok := installed[name]
					if !ok {
						missing = append(missing, name)
						continue
					}
					if rule != "" {
						converted, valid := pythonConstraint(rule)
						if !valid {
							skipped++
							continue
						}
						match, known := compatible(v, converted)
						if !known {
							skipped++
						} else if !match {
							mismatched = append(mismatched, name)
						}
					}
				}
				sort.Strings(missing)
				sort.Strings(mismatched)
				command := "python -m pip install ."
				if diagnostic.Exists(c.Files, "requirements.txt") {
					command = "python -m pip install -r requirements.txt"
				}
				if len(missing) > 0 {
					f = append(f, finding("python.dependencies.missing", diagnostic.Dependencies, diagnostic.Error, "Python packages missing from the local environment", "Not found in local package metadata: "+namesSummary(missing)+".", venv, repair("Activate the project's virtual environment; review packages and build hooks before installing.", command)))
				}
				if len(mismatched) > 0 {
					f = append(f, finding("python.dependencies.version", diagnostic.Dependencies, diagnostic.Error, "Python package versions do not match", "Installed metadata does not satisfy the supported requirement rules for: "+namesSummary(mismatched)+".", venv, repair("Activate the virtual environment and install the required package versions.", command)))
				}
				if len(missing) == 0 && len(mismatched) == 0 {
					f = append(f, finding("python.dependencies.present", diagnostic.Dependencies, diagnostic.Pass, "Parsed Python dependencies are present", "Matching local distribution metadata was found. Importability and native libraries are not tested.", venv))
				}
			}
		}
	}
	if skipped > 0 {
		f = append(f, finding("python.dependencies.partial", diagnostic.Dependencies, diagnostic.Info, "Some Python requirements need manual review", "Conditional markers, direct URLs, nested requirement files, complex Poetry rules, and unsupported versions are not evaluated. Their values are not exported.", ""))
	}
	for _, pair := range []struct{ file, tool string }{{"uv.lock", "uv"}, {"poetry.lock", "poetry"}, {"Pipfile", "pipenv"}} {
		if diagnostic.Exists(c.Files, pair.file) {
			_, tf := runtimeFinding(ctx, c, pair.tool, "python.tool."+pair.tool)
			tf.Category = diagnostic.Dependencies
			f = append(f, tf)
		}
	}
	if diagnostic.Exists(c.Files, "setup.py") || diagnostic.Exists(c.Files, "setup.cfg") {
		f = append(f, finding("python.legacy", diagnostic.Configuration, diagnostic.Info, "Legacy Python configuration detected", "setup.py is never executed. Dependencies declared only in setup.py or setup.cfg are not inspected.", ""))
	}
	return f
}

func pythonPackages(c *diagnostic.Context, venv string) (map[string]string, bool) {
	dirs := []string{filepath.Join(venv, "Lib", "site-packages")}
	for _, base := range []string{"lib", "lib64"} {
		if names, err := c.Files.List(filepath.Join(venv, base)); err == nil {
			for _, name := range names {
				if pyVersionDir.MatchString(name) {
					dirs = append(dirs, filepath.Join(venv, base, name, "site-packages"))
				}
			}
		}
	}
	installed := map[string]string{}
	readOK := false
	for _, dir := range dirs {
		names, err := c.Files.List(dir)
		if err != nil {
			continue
		}
		readOK = true
		for _, name := range names {
			if !strings.HasSuffix(name, ".dist-info") {
				continue
			}
			base := strings.TrimSuffix(name, ".dist-info")
			i := strings.LastIndex(base, "-")
			if i < 1 {
				continue
			}
			pkg, version := base[:i], base[i+1:]
			if pyRequirement.MatchString(pkg) && safeConstraint.MatchString(version) && diagnostic.IsDir(c.Files, filepath.Join(dir, name)) {
				installed[normalizePy(pkg)] = version
			}
		}
	}
	return installed, readOK
}
