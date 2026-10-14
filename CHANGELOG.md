# Changelog

## v0.1.0 — 2026-10-14

Initial public release of StackDoctor.

### Included

- Go application with a responsive Bubble Tea dashboard, Lip Gloss styling, Bubbles search/spinner/viewport components, category navigation, issue explanations, passed-check toggles, and rescans.
- `check`, `doctor`, `explain`, `report`, help, and version commands.
- Real Node.js, Python, Go, and Rust project detectors; runtime/version probes, local dependency clues, manifest checks, and review-only repair suggestions.
- Environment template filename and key-name checks without exported values.
- Explicit and inferred loopback TCP listener checks with a disable option.
- JSON/Markdown exports, clear CLI exit codes, and no-overwrite output files.
- Bounded project-scoped file access, special-file rejection, fixed probe allowlist, and secret-safe errors.
- Public compiled-in detector contract, contributor guidance, native Linux/Windows CI, four binary targets, and archive checksums.

### Limits

Workspace roots are not recursively scanned. Go/Rust global dependency availability is unverified. Python handles a common requirements subset and local environment metadata. Node checks package presence rather than transitive integrity or installed dependency versions. Port checks are point-in-time loopback TCP checks. `clear` does not guarantee startup. See the README for full boundaries.
