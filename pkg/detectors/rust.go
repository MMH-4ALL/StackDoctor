package detectors

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/MMH-4ALL/StackDoctor/pkg/diagnostic"
	"github.com/pelletier/go-toml/v2"
)

type Rust struct{}

func (Rust) ID() string { return "rust" }

type cargoManifest struct {
	Package *struct {
		Name        string `toml:"name"`
		Version     any    `toml:"version"`
		RustVersion any    `toml:"rust-version"`
		Edition     any    `toml:"edition"`
	} `toml:"package"`
	Workspace *struct {
		Members []string `toml:"members"`
	} `toml:"workspace"`
	Dependencies    map[string]any `toml:"dependencies"`
	DevDependencies map[string]any `toml:"dev-dependencies"`
}

func (Rust) Detect(ctx context.Context, c *diagnostic.Context) []diagnostic.Finding {
	if !diagnostic.Exists(c.Files, "Cargo.toml") {
		return nil
	}
	f := []diagnostic.Finding{}
	compiler, cf := runtimeFinding(ctx, c, "rustc", "rust.runtime")
	f = append(f, cf)
	_, cargo := runtimeFinding(ctx, c, "cargo", "rust.cargo")
	cargo.Category = diagnostic.Dependencies
	f = append(f, cargo)
	b, err := c.Files.Read("Cargo.toml")
	if err != nil {
		return append(f, unreadable("rust.manifest.unreadable", "Cargo.toml"))
	}
	var p cargoManifest
	if toml.Unmarshal(b, &p) != nil || (p.Package == nil && p.Workspace == nil) {
		return append(f, finding("rust.manifest.invalid", diagnostic.Configuration, diagnostic.Error, "Cargo.toml cannot be parsed", "Use valid TOML with a package or workspace table. Parser errors and values are not exported.", "Cargo.toml"))
	}
	f = append(f, finding("rust.manifest.valid", diagnostic.Configuration, diagnostic.Pass, "Cargo.toml parsed", "Rust project detected. Cargo commands and project build scripts are never run.", "Cargo.toml"))
	if p.Package != nil {
		if rule, ok := p.Package.RustVersion.(string); ok {
			if rule != "" && compiler.Available {
				f = append(f, versionFinding("rust.version.minimum", "Rust", compiler.Version, ">="+rule, "Cargo.toml"))
			}
		} else if p.Package.RustVersion != nil {
			f = append(f, finding("rust.version.inherited", diagnostic.Runtime, diagnostic.Info, "Inherited Rust version requirement is unverified", "A workspace-inherited requirement is not resolved by scanning parent manifests.", "Cargo.toml"))
		}
		if edition, ok := p.Package.Edition.(string); ok {
			switch edition {
			case "2015", "2018", "2021", "2024":
				if edition == "2024" && compiler.Available {
					f = append(f, versionFinding("rust.edition.version", "Rust", compiler.Version, ">=1.85.0", "Cargo.toml"))
				}
			default:
				f = append(f, finding("rust.edition.invalid", diagnostic.Configuration, diagnostic.Warning, "Cargo edition needs review", "The edition is not one of the known editions: 2015, 2018, 2021, or 2024.", "Cargo.toml"))
			}
		}
	}
	for _, file := range []string{"rust-toolchain.toml", "rust-toolchain"} {
		if !diagnostic.Exists(c.Files, file) {
			continue
		}
		b, err := c.Files.Read(file)
		if err != nil {
			f = append(f, unreadable("rust.toolchain.unreadable."+file, file))
			continue
		}
		pin := strings.TrimSpace(string(b))
		if file == "rust-toolchain.toml" {
			var tool struct {
				Toolchain struct {
					Channel string `toml:"channel"`
				} `toml:"toolchain"`
			}
			if toml.Unmarshal(b, &tool) != nil {
				f = append(f, finding("rust.toolchain.invalid", diagnostic.Configuration, diagnostic.Error, "rust-toolchain.toml cannot be parsed", "Correct its TOML syntax. StackDoctor does not ask rustup to install toolchains.", file))
				continue
			}
			pin = tool.Toolchain.Channel
		}
		if numericVersion.MatchString(pin) && compiler.Available {
			f = append(f, versionFinding("rust.toolchain.version."+file, "Rust", compiler.Version, pin, file))
		} else {
			f = append(f, finding("rust.toolchain.channel."+file, diagnostic.Runtime, diagnostic.Info, "Rust toolchain channel needs manual review", "Moving stable, beta, and nightly channels are not resolved offline. No toolchain is downloaded.", file))
		}
	}
	missing, external := false, false
	for _, deps := range []map[string]any{p.Dependencies, p.DevDependencies} {
		for _, value := range deps {
			if table, ok := value.(map[string]any); ok {
				if path, ok := table["path"].(string); ok {
					if !diagnostic.Within(".", path) {
						external = true
					} else if !diagnostic.Exists(c.Files, filepath.Join(path, "Cargo.toml")) {
						missing = true
					}
				}
			}
		}
	}
	if missing {
		f = append(f, finding("rust.dependencies.path.missing", diagnostic.Dependencies, diagnostic.Error, "Local Rust dependency is unavailable", "An in-project path dependency has no readable Cargo.toml.", "Cargo.toml"))
	}
	if external {
		f = append(f, finding("rust.dependencies.path.external", diagnostic.Configuration, diagnostic.Warning, "Rust dependency points outside this project", "External path dependencies are not inspected. Select the target explicitly to scan it.", "Cargo.toml"))
	}
	if p.Package != nil || len(p.Dependencies) > 0 {
		if !diagnostic.Exists(c.Files, "Cargo.lock") {
			f = append(f, finding("rust.lock.missing", diagnostic.Dependencies, diagnostic.Warning, "Cargo.lock is missing", "This can be intentional for a library; applications usually keep a lockfile. Registry and Git dependencies are not checked.", "Cargo.toml", repair("Review dependency sources, then fetch if needed. This command uses the network.", "cargo fetch")))
		} else {
			b, err := c.Files.Read("Cargo.lock")
			var lock map[string]any
			if err != nil {
				f = append(f, unreadable("rust.lock.unreadable", "Cargo.lock"))
			} else if toml.Unmarshal(b, &lock) != nil {
				f = append(f, finding("rust.lock.invalid", diagnostic.Configuration, diagnostic.Error, "Cargo.lock cannot be parsed", "Correct or regenerate the TOML lockfile after reviewing dependencies.", "Cargo.lock"))
			}
		}
		f = append(f, finding("rust.dependencies.cache", diagnostic.Dependencies, diagnostic.Info, "Rust dependency availability is unverified", "Global Cargo caches are not scanned. No cargo metadata, cargo check, or build.rs code is executed.", "Cargo.toml"))
	}
	if p.Workspace != nil {
		f = append(f, finding("rust.workspace", diagnostic.Configuration, diagnostic.Info, "Cargo workspace detected", "Only the selected manifest is inspected. Use --path to check individual workspace members.", "Cargo.toml"))
	}
	return f
}
