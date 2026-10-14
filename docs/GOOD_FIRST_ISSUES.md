# Good first issues

These are proposed extensions, not features of v0.1.0. Check the issue tracker for ownership and status before starting.

## 1. Recognize `.env.dist` as an environment template

Touch `pkg/detectors/env.go` and its tests. Map `.env.dist` to `.env` using the same rules as `.env.example`.

Acceptance criteria:

- A missing target produces a warning naming `.env`, with a review-only suggestion.
- An existing target is checked by key name only.
- Template and actual secret sentinel values never appear in a report.
- Existing targets are not changed, paths remain bounded, and README scope is updated.

## 2. Inspect root `requirements-dev.txt`

Extend the Python detector to inspect a literal `requirements-dev.txt` in the selected root using its existing requirement parser. Do not follow `-r` includes or external environments.

Acceptance criteria:

- Missing parsed development packages in an existing local environment are diagnosed.
- Conditional/URL/include requirements are still reported as unverified without leaking values.
- Absence of the file does not change existing results.
- Tests cover missing, present, malformed/conditional, and secret-sentinel cases.

## 3. Explain npm engine requirements

The Node detector already probes the selected package manager. Add a check for `engines.npm` when npm is selected and its version is known; do not add any new subprocess commands.

Acceptance criteria:

- Supported matching/mismatching ranges produce the correct result.
- Unknown or unsafe ranges produce a review warning without echoing unsafe input.
- Shell-wrapper version uncertainty remains explicit on Windows.
- Stable unique finding IDs, tests, and README documentation are included.
