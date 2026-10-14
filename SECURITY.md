# Security policy

## Supported versions

Security fixes are intended for the latest published release. StackDoctor's first supported release is `v0.1.0`.

## Private reports

Use [GitHub's private vulnerability reporting page](https://github.com/MMH-4ALL/StackDoctor/security/advisories/new) when it is available. Otherwise use a private contact method listed on [the maintainer's profile](https://github.com/MMH-4ALL), or open a neutral issue requesting one. Do not publish an exploit, credentials, or affected private material in an ordinary issue.

Include the version, OS, a safe reproducer, expected boundary, and actual behavior. Report examples with invented values, not live secrets. Response times depend on maintainer availability; this project does not promise a staffed security service.

## Threat model

Project manifests and environment files are untrusted input. StackDoctor is designed to diagnose them without executing project code or leaking their secret values.

The selected directory is opened through `os.OpenRoot`. Reads use bounded traversal-resistant operations, reject final symbolic links and special files, and cannot follow intermediate symlinks outside the root. There is no automatic parent or recursive workspace scan. External replacements and workspace targets are reported without being opened.

Environment parsing retains key names; only a strictly numeric `PORT` is used for port probes. Reports contain fixed diagnostic explanations and validated names/versions, not secret values, raw parser errors, raw runtime output, scripts, URLs, or full paths.

Runtime discovery uses trusted executables on `PATH` outside the selected project. Only fixed version probes are allowed. The working directory is the system temporary directory, project-local executables are refused, common injection variables are stripped, automatic Go/Corepack/Rustup downloads are disabled where supported, and output/time are bounded. Windows shell wrappers are not run to obtain versions.

StackDoctor cannot sandbox a malicious global executable, an already-compromised OS, or a network-mounted filesystem with hostile behavior. Runtime executables may perform their own internal configuration reads. Treat `PATH` as part of your trusted environment. Compiled-in third-party detectors are trusted application code and must follow the contributor contract.

Loopback TCP checks connect briefly to selected local ports. They do not send application data, inspect process command lines, kill listeners, or contact remote addresses. Use `--no-ports` to disable connections.

Repair instructions are text only. The application has no repair executor. Suggested package-manager commands may download packages and run build/install hooks when **you** execute them; review the project's sources and manifests first.

## Reports and releases

Before sharing a report, review its project name, key names, versions, and port numbers. Those can be private context even though values are excluded. `--output` creates a new file and never replaces an existing one; Unix file mode is `0600` subject to platform semantics.

Release archives include SHA-256 checksums. Checksums detect corruption; Windows binaries are not Authenticode signed. Dependencies are tracked in `go.mod`/`go.sum`, and CI checks native Linux/Windows tests before release packaging.
