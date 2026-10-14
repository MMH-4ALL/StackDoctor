# Contributing to StackDoctor

Small, tested improvements are welcome. A useful diagnostic must explain both what was checked and what remains unknown.

## Start here

1. Check existing issues and pull requests. For a new detector or a large behavior change, open an issue describing the concrete project failure first.
2. Fork and clone the repository. Install Go 1.24 or later.
3. Create a branch and make a focused change. Keep unrelated formatting and refactors out of the patch.
4. Add a test that demonstrates the real failure and its expected diagnosis.
5. Run the checks below and submit a pull request using the template.

```bash
gofmt -w cmd internal pkg
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/stackdoctor
```

CI also tests Windows and builds Linux/Windows x86-64 and ARM64. You can package every target with `python3 scripts/package.py 0.1.0`; Python is a maintainer dependency, not an application runtime dependency.

## Adding a detector

Read [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). Implement `diagnostic.Detector`, use `Context.Files` for every project read, and register it in `detectors.Builtins`. Give findings unique, stable IDs so `explain` and JSON consumers can address them.

Required boundaries:

- Never execute project code, install packages, auto-run repairs, or load project-supplied executable plugins.
- Do not call `os.ReadFile`, recurse through the filesystem, inspect global caches, or follow external paths from a detector. Use the bounded `Files` interface.
- Never return environment values, full raw manifests, arbitrary scripts, dependency URLs, raw parser errors, or unfiltered process output. Prefer fixed explanations, validated key names, and supported version numbers.
- Describe uncertain checks as `info` or `warning`, not a false pass or a made-up score.
- Keep commands display-only and fixed in code. Explain install/build/network side effects in the repair description. Avoid destructive commands.
- Use fake tool/port providers in detector tests. Keep real OS checks narrow and deterministic.
- Cover malformed inputs, missing prerequisites, and privacy or path boundaries relevant to the change.

Document the detector's scope and limitations in the README. No required accounts or paid services should be introduced.

## Reporting bugs

Use the bug template. Include the StackDoctor version, OS, command, expected behavior, and a **sanitized** minimal manifest. Review reports before sharing: key names and project names can still be private even though secret values are excluded.

Report a suspected secret disclosure or path-boundary bypass privately, following [SECURITY.md](SECURITY.md).

## First contributions

[GOOD_FIRST_ISSUES.md](docs/GOOD_FIRST_ISSUES.md) provides small, concrete tasks with acceptance criteria. Ask on the relevant issue if scope is unclear; a short reproducer often helps more than a large rewrite.
