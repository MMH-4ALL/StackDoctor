package detectors

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/MMH-4ALL/StackDoctor/pkg/diagnostic"
)

type Node struct{}

func (Node) ID() string { return "node" }

type nodeManifest struct {
	Engines         map[string]string `json:"engines"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Scripts         map[string]string `json:"scripts"`
	PackageManager  string            `json:"packageManager"`
	Workspaces      json.RawMessage   `json:"workspaces"`
}

var packageName = regexp.MustCompile(`^(?:@[a-zA-Z0-9_.-]+/)?[a-zA-Z0-9_.-]+$`)
var managerDeclaration = regexp.MustCompile(`^(npm|pnpm|yarn|bun)@([0-9]+\.[0-9]+\.[0-9]+)(?:\+[a-zA-Z0-9.]+)?$`)

func readNode(c *diagnostic.Context) (nodeManifest, error) {
	b, err := c.Files.Read("package.json")
	if err != nil {
		return nodeManifest{}, err
	}
	var p nodeManifest
	err = json.Unmarshal(b, &p)
	if err == nil && strings.TrimSpace(string(b)) == "null" {
		err = &json.SyntaxError{}
	}
	return p, err
}

func nodeManager(c *diagnostic.Context, p nodeManifest) (string, []string) {
	locks := []string{}
	managers := []string{}
	for _, pair := range []struct{ file, manager string }{{"package-lock.json", "npm"}, {"npm-shrinkwrap.json", "npm"}, {"pnpm-lock.yaml", "pnpm"}, {"yarn.lock", "yarn"}, {"bun.lock", "bun"}, {"bun.lockb", "bun"}} {
		if diagnostic.Exists(c.Files, pair.file) {
			locks = append(locks, pair.file)
			if !contains(managers, pair.manager) {
				managers = append(managers, pair.manager)
			}
		}
	}
	if m := managerDeclaration.FindStringSubmatch(p.PackageManager); len(m) > 0 {
		return m[1], locks
	}
	if len(managers) == 1 {
		return managers[0], locks
	}
	return "npm", locks
}

func contains(list []string, s string) bool {
	for _, i := range list {
		if i == s {
			return true
		}
	}
	return false
}

func (Node) Detect(ctx context.Context, c *diagnostic.Context) []diagnostic.Finding {
	if !diagnostic.Exists(c.Files, "package.json") {
		return nil
	}
	p, err := readNode(c)
	if err != nil {
		return []diagnostic.Finding{finding("node.manifest.invalid", diagnostic.Configuration, diagnostic.Error, "package.json cannot be parsed", "Use valid JSON with an object at the root and string values for engines, dependencies, and scripts. Parser errors and manifest values are not echoed.", "package.json")}
	}
	f := []diagnostic.Finding{finding("node.manifest.valid", diagnostic.Configuration, diagnostic.Pass, "package.json parsed", "Node.js project detected.", "package.json")}
	t, runtime := runtimeFinding(ctx, c, "node", "node.runtime")
	f = append(f, runtime)
	if rule := p.Engines["node"]; rule != "" && t.Available {
		f = append(f, versionFinding("node.version.engines", "Node.js", t.Version, rule, "package.json"))
	}
	for _, file := range []string{".nvmrc", ".node-version"} {
		if !diagnostic.Exists(c.Files, file) {
			continue
		}
		b, err := c.Files.Read(file)
		if err != nil {
			f = append(f, unreadable("node.version.file."+file, file))
			continue
		}
		rule := strings.TrimSpace(string(b))
		if !numericVersion.MatchString(rule) {
			f = append(f, finding("node.version."+file, diagnostic.Runtime, diagnostic.Info, "Node version alias needs manual review", "Only numeric version pins are checked offline. Version-manager aliases are not resolved.", file))
			continue
		}
		if t.Available {
			f = append(f, versionFinding("node.version."+file, "Node.js", t.Version, rule, file))
		}
	}
	manager, locks := nodeManager(c, p)
	mt, mf := runtimeFinding(ctx, c, manager, "node.manager")
	mf.Category = diagnostic.Dependencies
	f = append(f, mf)
	if p.PackageManager != "" {
		if m := managerDeclaration.FindStringSubmatch(p.PackageManager); len(m) > 0 {
			if mt.Version != "" {
				f = append(f, versionFinding("node.manager.version", manager, mt.Version, m[2], "package.json"))
			}
		} else {
			f = append(f, finding("node.manager.declaration", diagnostic.Configuration, diagnostic.Warning, "packageManager declaration needs review", "Expected npm, pnpm, yarn, or bun followed by a pinned semantic version.", "package.json"))
		}
	}
	lockManagers := map[string]bool{}
	for _, lock := range locks {
		switch lock {
		case "package-lock.json", "npm-shrinkwrap.json":
			lockManagers["npm"] = true
		case "pnpm-lock.yaml":
			lockManagers["pnpm"] = true
		case "yarn.lock":
			lockManagers["yarn"] = true
		default:
			lockManagers["bun"] = true
		}
	}
	if len(lockManagers) > 1 {
		f = append(f, finding("node.locks.conflict", diagnostic.Configuration, diagnostic.Warning, "Multiple package-manager lockfiles", "Found "+strings.Join(locks, ", ")+". Confirm the intended package manager before installing.", "package.json"))
	}
	if len(lockManagers) == 1 && !lockManagers[manager] {
		f = append(f, finding("node.locks.manager", diagnostic.Configuration, diagnostic.Warning, "Lockfile and packageManager disagree", "The declared package manager does not match the detected lockfile.", "package.json"))
	}
	if len(locks) == 0 && (len(p.Dependencies)+len(p.DevDependencies) > 0) {
		f = append(f, finding("node.locks.missing", diagnostic.Configuration, diagnostic.Warning, "No package lockfile", "Dependency versions may vary between installs. Use the project's intended package manager to generate a lockfile.", "package.json"))
	}
	command := manager + " install"
	if manager == "npm" && (diagnostic.Exists(c.Files, "package-lock.json") || diagnostic.Exists(c.Files, "npm-shrinkwrap.json")) {
		command = "npm ci"
	}
	if manager == "pnpm" && diagnostic.Exists(c.Files, "pnpm-lock.yaml") {
		command = "pnpm install --frozen-lockfile"
	}
	if manager == "yarn" {
		command = "yarn install"
	}
	if manager == "bun" && len(locks) > 0 {
		command = "bun install --frozen-lockfile"
	}
	// Presence checks work with npm/pnpm's linked layout. PnP has no node_modules.
	deps := map[string]string{}
	for name, v := range p.Dependencies {
		deps[name] = v
	}
	for name, v := range p.DevDependencies {
		deps[name] = v
	}
	if diagnostic.Exists(c.Files, ".pnp.cjs") || diagnostic.Exists(c.Files, ".pnp.js") {
		f = append(f, finding("node.dependencies.pnp", diagnostic.Dependencies, diagnostic.Info, "Yarn Plug'n'Play detected", "A PnP loader exists. Individual package availability is not verified without executing it.", ".pnp.cjs"))
	} else if len(deps) > 0 {
		missing := []string{}
		unsupported := 0
		for name := range deps {
			if !packageName.MatchString(name) || name == "." || name == ".." {
				unsupported++
				continue
			}
			// Stat refuses external symlinks. A pnpm package path containing an
			// internal symlink remains confined by os.Root.
			if _, err := c.Files.Stat(filepath.Join("node_modules", filepath.FromSlash(name), "package.json")); err != nil {
				missing = append(missing, name)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			f = append(f, finding("node.dependencies.missing", diagnostic.Dependencies, diagnostic.Error, "Declared Node packages are missing", "Not found inside this project: "+namesSummary(missing)+". This includes development dependencies.", "package.json", repair("Review install scripts and the lockfile before installing project dependencies.", command)))
		} else if unsupported == 0 {
			f = append(f, finding("node.dependencies.present", diagnostic.Dependencies, diagnostic.Pass, "Declared Node packages are present", "Package manifests exist in this project's node_modules. Transitive integrity and installed package versions are not checked.", "node_modules"))
		}
		if unsupported > 0 {
			f = append(f, finding("node.dependencies.unsupported", diagnostic.Dependencies, diagnostic.Warning, "Some dependency names could not be checked", "Invalid or unsafe package names were ignored.", "package.json"))
		}
	}
	if len(p.Workspaces) > 0 && string(p.Workspaces) != "null" {
		f = append(f, finding("node.workspaces", diagnostic.Configuration, diagnostic.Info, "Workspace project detected", "Only this directory is checked. Select a workspace with --path to inspect its manifest; parent folders are not scanned.", "package.json"))
	}
	if _, dev := p.Scripts["dev"]; !dev {
		if _, start := p.Scripts["start"]; !start {
			f = append(f, finding("node.scripts", diagnostic.Configuration, diagnostic.Info, "No dev or start script", "This may be a library. Inspect package.json for the intended entry point.", "package.json"))
		}
	}
	if diagnostic.Exists(c.Files, "package-lock.json") {
		b, err := c.Files.Read("package-lock.json")
		var lock struct {
			LockfileVersion int `json:"lockfileVersion"`
		}
		if err != nil {
			f = append(f, unreadable("node.lock.unreadable", "package-lock.json"))
		} else if json.Unmarshal(b, &lock) != nil || lock.LockfileVersion < 1 || lock.LockfileVersion > 3 {
			f = append(f, finding("node.lock.invalid", diagnostic.Configuration, diagnostic.Error, "package-lock.json is invalid", "The lockfile must be valid JSON with a supported lockfileVersion.", "package-lock.json"))
		}
	}
	return f
}
