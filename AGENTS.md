# AGENTS.md

Guidance for coding agents operating in this repository.

## Scope and priority

- Follow explicit user instructions first.
- Then follow this AGENTS.md.

## Repository snapshot

- Language: Go
- Module: `xci` (see `go.mod`)
- Go version in module: `1.25.5`
- Main entrypoint: `src/main.go`
- Internal packages: `src/tools`, `src/version`
- Optional task runner config: `ror.kdl`
- Build output directory: `out/` (gitignored)

## Build, run, lint, and test commands

### Build commands

- Primary build command (also to check compiling):
  - `ror ai:build`

### Run commands

- Run without prebuilding:
  - `go ai:run`
- Print version:
  - `go ai:run version`

### Test commands

- Run all tests in module:
  - `go test ./...`
- Run tests for one package:
  - `go test ./src/tools`
- Run a single test function (most important pattern):
  - `go test ./src/tools -run '^TestInstall$'`
- Run a subset by regex:
  - `go test ./src/tools -run 'TestInstall|TestUpdate'`
- Run single test with verbose output and no cache:
  - `go test ./src/tools -run '^TestInstall$' -v -count=1`

### Lint and formatting commands

- Format all changed Go files with `gofmt` before finalizing edits.
- Common full-repo formatting pattern:
  - `gofmt -w ./src`
- Static analysis:
  - `go vet ./...`
- Compile + test gate often used as lightweight CI equivalent:
  - `go test ./... && go build ./...`

## Coding style guidelines

### General Go conventions

- Follow idiomatic Go and keep code `gofmt`-clean.
- Keep functions focused and small where practical.
- Prefer standard library solutions over adding new dependencies.
- Keep package APIs minimal; export only what is needed.

### Imports

- Use normal Go import blocks and let `gofmt` sort/import-format.
- Use module-qualified internal imports (current pattern):
  - `xci/src/tools`
  - `xci/src/version`
- Avoid unused imports; remove them immediately.

### Formatting

- Always run `gofmt` on edited files.
- Do not hand-align spacing; `gofmt` is the source of truth.
- Keep lines readable; if a call is dense, split arguments across lines.

### Types and data design

- Prefer concrete types unless an interface is required by behavior boundaries.
- Keep structs and fields unexported by default.
- Export identifiers only for cross-package APIs.
- Use zero values deliberately and document non-obvious invariants.

### Naming conventions

- Use `camelCase` for unexported names.
- Use `PascalCase` for exported names.
- Use short receiver names that are consistent and meaningful.
- Keep acronyms consistent (existing code uses `XCI` in helper names).
- Prefer descriptive function names for side-effecting operations.

### Error handling

- Return `error` values instead of panicking for expected failures.
- Wrap underlying errors with context using `%w`:
  - `fmt.Errorf("failed to copy file: %w", err)`
- Use clear, action-oriented error messages.
- At CLI boundary (`main`), print user-facing errors to `stderr` and exit non-zero.
- Reserve panic for impossible invariants only.

### CLI and process execution

- Use `exec.Command` with explicit args (no shell string parsing).
- For interactive subprocesses, wire `stdin/stdout/stderr` explicitly.
- For preview/plan commands, capture output when confirmation is needed.
- Keep user prompts concise and deterministic (`y/n` style used in repo).

### File system safety

- Prefer atomic file replacement patterns for installs/updates:
  - write temp file
  - close file
  - rename into place
- Re-check executable permissions after copy when needed.
- Avoid overwriting unknown binaries/files without positive identification.

### Comments and docs

- Add comments for exported symbols.
- Use comments to explain non-obvious intent, not obvious mechanics.
- Keep comments accurate during refactors; remove stale comments promptly.

## Expected agent workflow for edits

- Read nearby code before editing to match local patterns.
- Make the smallest safe change that satisfies the request.
- Run `gofmt` on touched Go files.
- Run focused verification first, then broader checks:
  - package-level `go test`
  - then `go test ./...`
  - then `go build ./...` for final compile confidence
- If tests do not exist for changed behavior, add targeted tests when feasible.

## Change and review checklist

- Code compiles: `go build ./...`
- Tests pass: `go test ./...`
- Formatting clean: `gofmt` applied
- Errors wrapped with context where appropriate
- No accidental API exports
- No unrelated file churn

## Notes for future maintainers

- If repo tooling changes (Makefile, Taskfile, CI config, linters), update this file.
- If Cursor/Copilot rule files are added, add a dedicated section summarizing them.
- Keep this guide practical and command-accurate over time.
