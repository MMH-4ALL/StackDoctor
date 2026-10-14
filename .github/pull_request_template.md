## Problem and change

Describe the concrete failure and resulting behavior. Link the issue if one exists.

## Verification

State the meaningful tests or reproducer you ran and their results.

- [ ] `go test ./...`
- [ ] `go test -race ./...`
- [ ] `go vet ./...`
- [ ] Formatting and README scope updated where needed

## Boundaries

- [ ] No project code or repair commands execute automatically
- [ ] File reads use the project-scoped Files interface
- [ ] No environment values, raw parser output, scripts, or private URLs enter findings
- [ ] Findings state uncertainty and use unique, stable IDs

List known limitations relevant to the change.
