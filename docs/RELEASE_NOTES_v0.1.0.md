# StackDoctor v0.1.0

Find what is blocking a cloned project without running its code.

StackDoctor's initial release includes a working full-screen terminal dashboard and a scriptable CLI for Node.js, Python, Go, and Rust. It reports detected runtime/version, local dependency, environment-template, port-listener, and configuration problems, then explains repair commands for you to review.

## Download

Choose Linux or Windows and your CPU architecture: `amd64` for x86-64 or `arm64`. Archives include the executable, README, MIT license, and these notes. `checksums.txt` contains SHA-256 checksums. No Go installation is needed for a release binary.

```bash
stackdoctor check
stackdoctor doctor
stackdoctor report --format json --output diagnosis.json
```

## Privacy and scope

No accounts, telemetry, required paid APIs, or automatic repair execution. File reads stay within the selected project and are bounded. Environment values and raw parser/tool output are excluded from diagnostics. External runtime executables receive fixed version-only probes; optional port checks use loopback TCP.

## Known limitations

Select workspace members individually. Go/Rust global caches are not checked. Python inspects a common requirements subset and local metadata; external environments are not covered. Yarn PnP and Windows package-manager shell wrappers are recognized without executing their loaders/wrappers. Port results are point-in-time TCP checks, not bind guarantees. Environment-template keys can be optional.

This release favors explicit, bounded checks over claiming complete framework coverage. See the README and SECURITY.md for the full check matrix and threat model.
