# Architecture

The CLI and terminal dashboard are two views of the same real diagnostic report. No UI code invents findings or health values.

```text
cmd/stackdoctor      process entry point and build version
internal/cli        command parsing, exit codes, output-file creation
internal/tui        Bubble Tea state, navigation, search, viewport, rendering
pkg/diagnostic      report types, registry, bounded files, runtime/port providers
pkg/detectors       explicit built-in registrations and stack-specific checks
pkg/report          JSON and Markdown rendering
scripts             release packaging
docs                design contract, release notes, authentic terminal capture
```

## Scan flow

1. The caller selects one directory and a timeout.
2. `Registry.Scan` opens it with `os.OpenRoot`, detects root manifest markers, and builds a context containing bounded file access, a cached runtime provider, and a loopback port provider.
3. Detectors run in registration order. They parse known project files, probe fixed external runtime commands, and return typed findings.
4. The engine adds detector IDs, strips terminal controls from presentation text, sorts findings by severity and stable ID, and derives health from actual counts.
5. The caller renders text, JSON, Markdown, or a dashboard. Repairs are data, never executable callbacks.

The dashboard starts the scan asynchronously through a Bubble Tea command so it can show progress and respond to quit. Rescan creates a fresh runtime cache. Scan results are snapshots rather than an active file watcher.

## Detector contract

```go
type Detector interface {
    ID() string
    Detect(context.Context, *diagnostic.Context) []diagnostic.Finding
}
```

`Context.Files` offers `Read`, `Stat`, and `List` inside the root. It supplies no recursive walk or arbitrary global access. `Context.Tools` permits fixed cached version probes. `Context.Ports` checks selected loopback ports and can be disabled.

To add a framework detector:

1. Add a type in `pkg/detectors` with a stable detector ID.
2. Gate it on a known marker using `Context.Files`. Do not execute config files to discover their values.
3. Return findings with globally unique stable IDs, a category, severity, fixed explanation, optional safe file name, and optional display-only repair suggestions.
4. Register it in `Builtins`. Add tests using fake tool/port providers and temporary project fixtures.
5. Update the README with exactly what the detector can prove and what it cannot.

This is a **compiled-in, plugin-like extension system**. Contributors do not need to rewrite the CLI/UI. Dynamic shared libraries, subprocess plugins, and project-downloaded detectors are deliberately excluded from this release.

## Finding semantics

| Severity | Meaning |
| --- | --- |
| `error` | A concrete detected blocker, such as a missing runtime or invalid manifest |
| `warning` | A likely problem or check requiring review, such as an absent template target |
| `info` | Context or an explicit boundary/unknown |
| `pass` | The named narrow check succeeded |

Health is `blocked` if there are errors, `needs attention` if only warnings exist, `clear` if a stack was detected without errors/warnings, and `unknown` if no stack was detected without issues. Informational unknowns remain visible even when health is clear.

JSON has `schema_version: 1`. Consumers should tolerate additional fields and unknown finding IDs. Human text is not a stable parsing API; use JSON. Report exports contain the project directory name, not an absolute path.

## Safety rules

Do not rely on escaping as secret redaction. Detectors must never return secret-bearing values. The engine strips terminal controls; report Markdown additionally escapes markup. Safe messages are constructed at the detector boundary, and raw parsing/probe failures are replaced with fixed explanations.

File reads are capped at 2 MiB. Directory lists stop at 4,096 entries. Final symlinks and non-regular file reads are rejected; `os.Root` prevents intermediate traversal outside the project. Linux opens reads without blocking on a substituted FIFO. Windows uses the platform's root implementation.

Runtime probes reject executables inside the selected project, never use a shell, remove common injection variables, use a neutral temporary working directory, and enforce three-second timeouts and 8 KiB output caps. `.cmd`/`.bat` tools on Windows are recognized without execution. Only extracted version strings reach findings. Go/Corepack/Rustup automatic download behavior is disabled where supported.

Contributor-added code remains trusted application code; the interface is a design boundary, not a process sandbox. See SECURITY.md for the complete threat model.

## Tests and release process

Unit tests cover detector fixtures, malformed manifests, secret sentinels, scan scope, special files, command parsing/exit codes, no-overwrite exports, report failures, and dashboard interactions/layout bounds. A real loopback listener test checks the OS port provider.

CI runs tests and vet on native Linux/Windows runners, plus four cross-build targets. The release workflow tests the tagged source on both OSes, packages Linux/Windows amd64/arm64, uploads archives and checksums to a draft release, then publishes it only after all uploads succeed.
