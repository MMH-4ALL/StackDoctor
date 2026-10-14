package detectors

import (
	"context"
	"strings"

	"github.com/MMH-4ALL/StackDoctor/pkg/diagnostic"
	"golang.org/x/mod/modfile"
)

type Go struct{}

func (Go) ID() string { return "go" }

func (Go) Detect(ctx context.Context, c *diagnostic.Context) []diagnostic.Finding {
	if !diagnostic.Exists(c.Files, "go.mod") && !diagnostic.Exists(c.Files, "go.work") {
		return nil
	}
	t, tf := runtimeFinding(ctx, c, "go", "go.runtime")
	f := []diagnostic.Finding{tf}
	if diagnostic.Exists(c.Files, "go.mod") {
		b, err := c.Files.Read("go.mod")
		if err != nil {
			f = append(f, unreadable("go.manifest.unreadable", "go.mod"))
			return f
		}
		p, err := modfile.Parse("go.mod", b, nil)
		if err != nil || p.Module == nil {
			f = append(f, finding("go.manifest.invalid", diagnostic.Configuration, diagnostic.Error, "go.mod cannot be parsed", "Correct the module directive and Go module syntax. Raw parser errors are not exported.", "go.mod"))
			return f
		}
		f = append(f, finding("go.manifest.valid", diagnostic.Configuration, diagnostic.Pass, "go.mod parsed", "Go module detected. Module cache and parent workspace files are not inspected.", "go.mod"))
		if p.Go != nil && t.Available {
			f = append(f, versionFinding("go.version.minimum", "Go", t.Version, ">="+p.Go.Version, "go.mod"))
		}
		if p.Toolchain != nil && t.Available {
			pin := strings.TrimPrefix(p.Toolchain.Name, "go")
			if numericVersion.MatchString(pin) {
				f = append(f, versionFinding("go.version.toolchain", "Go toolchain", t.Version, ">="+pin, "go.mod"))
			}
		}
		if len(p.Require) > 0 {
			if diagnostic.IsDir(c.Files, "vendor") {
				if diagnostic.Exists(c.Files, "vendor/modules.txt") {
					f = append(f, finding("go.dependencies.vendor", diagnostic.Dependencies, diagnostic.Info, "Vendored module metadata found", "vendor/modules.txt exists. Vendored contents and consistency are not verified.", "vendor/modules.txt"))
				} else {
					f = append(f, finding("go.dependencies.vendor.invalid", diagnostic.Dependencies, diagnostic.Warning, "Vendor directory has no module metadata", "Regenerate vendored dependencies after reviewing the module graph.", "vendor", repair("Review and regenerate the vendor directory.", "go mod vendor")))
				}
			} else if !diagnostic.Exists(c.Files, "go.sum") {
				f = append(f, finding("go.dependencies.sum", diagnostic.Dependencies, diagnostic.Warning, "go.sum is missing", "Required modules are declared, but no checksum file is present. This can be valid for local-only replacements; external module availability is not checked.", "go.mod", repair("Review the module graph; download dependencies if needed. This command uses the network.", "go mod download")))
			}
			f = append(f, finding("go.dependencies.cache", diagnostic.Dependencies, diagnostic.Info, "Go module availability is unverified", "StackDoctor does not read the global module cache or run go list. Review dependencies before using go mod download.", "go.mod"))
		}
		externalReplace, missingReplace := false, false
		for _, replace := range p.Replace {
			if replace.New.Version != "" {
				continue
			}
			path := replace.New.Path
			if strings.HasPrefix(path, ".") || strings.HasPrefix(path, "/") || strings.Contains(path, "\\") {
				if !diagnostic.Within(".", path) {
					externalReplace = true
				} else if !diagnostic.IsDir(c.Files, path) {
					missingReplace = true
				}
			}
		}
		if externalReplace {
			f = append(f, finding("go.replace.external", diagnostic.Configuration, diagnostic.Warning, "Local replacement points outside this project", "External replace targets were not inspected. Select each target directory separately if you want to check it.", "go.mod"))
		}
		if missingReplace {
			f = append(f, finding("go.replace.missing", diagnostic.Configuration, diagnostic.Error, "Local replacement directory is unavailable", "An in-project replace target is missing or cannot be accessed safely.", "go.mod"))
		}
	}
	if diagnostic.Exists(c.Files, "go.work") {
		b, err := c.Files.Read("go.work")
		if err != nil {
			f = append(f, unreadable("go.workspace.unreadable", "go.work"))
			return f
		}
		p, err := modfile.ParseWork("go.work", b, nil)
		if err != nil {
			f = append(f, finding("go.workspace.invalid", diagnostic.Configuration, diagnostic.Error, "go.work cannot be parsed", "Correct the Go workspace syntax. Raw parser errors are not exported.", "go.work"))
			return f
		}
		if p.Go != nil && t.Available {
			f = append(f, versionFinding("go.workspace.version", "Go", t.Version, ">="+p.Go.Version, "go.work"))
		}
		missing, external := false, false
		for _, use := range p.Use {
			if !diagnostic.Within(".", use.Path) {
				external = true
			} else if !diagnostic.IsDir(c.Files, use.Path) {
				missing = true
			}
		}
		if missing {
			f = append(f, finding("go.workspace.missing", diagnostic.Configuration, diagnostic.Error, "Go workspace member is unavailable", "An in-project use directory is missing or unsafe.", "go.work"))
		}
		if external {
			f = append(f, finding("go.workspace.external", diagnostic.Configuration, diagnostic.Warning, "Go workspace uses external directories", "External workspace members are not inspected. Select each member explicitly.", "go.work"))
		}
		f = append(f, finding("go.workspace.scope", diagnostic.Configuration, diagnostic.Info, "Go workspace detected", "Member manifests are not scanned automatically. Use --path for each module.", "go.work"))
	}
	return f
}
