# Contributing to OCI CPU Shaper

OCI CPU Shaper ships deterministic tooling so local changes line up with CI. This
guide expands on the notes in `README.md` and [`docs/08-development.md`](docs/08-development.md)
and should be read before sending a pull request.

## Getting Started

1. Discuss substantial changes in an issue before opening a pull request so the
   architecture remains aligned with `docs/initial-implementation-plan.md`.
2. Install the pinned toolchain (Go 1.24.10, `golangci-lint` 2.6.1,
   `gofumpt` 0.9.2) with `make tools` or `mise install` as described in
   §14 of `docs/08-development.md`.
3. Configure Git hooks via `git config core.hooksPath .githooks` if you want the
   optional pre-push checks that run `make fmt` and `make lint` for you.

## Tooling Workflow

Run the following helpers from the repository root before every pull request:

- `make fmt` to normalize Go formatting with `gofmt` and `gofumpt`.
- `make lint` to execute `golangci-lint` with auto-fix enabled. The target pins
  `GOLANGCI_LINT_CACHE` to `.cache/golangci` and mirrors the CI configuration.
- `make test` to run `go test -race ./...` so race conditions surface early.
- `make coverage MIN_COVERAGE=95` to regenerate `coverage.out`/`coverage.txt` and
  prove repository-wide statement coverage stays at or above the required 95 %.

Use `make check` when you want linting and race-enabled tests in one step. Run
`make integration`, `make e2e`, or `make bench` whenever you touch the packages
or workflows they cover (see the command table in `docs/08-development.md` for a
full description). Always re-run the relevant command after addressing feedback
so CI sees the final state you validated locally.

## Documentation Expectations

- Update `docs/CHANGELOG.md`, the impacted guide(s) under `docs/`, and any
  related READMEs/config samples whenever behaviour, interfaces, or defaults
  change. Keep headings aligned with the numbered sections in the docs plan.
- When coverage meaningfully increases or decreases, call it out in
  `docs/CHANGELOG.md` and explain the tests that caused the shift.
- Summarise manual verification steps in the pull request description whenever a
  scenario cannot be automated yet.

## Scoped `AGENTS.md` Policy

Every directory tree inherits rules from the nearest `AGENTS.md`. When you add
or move files:

1. Identify which scope governs the directory and update it if the rules no
   longer apply.
2. Add new `AGENTS.md` files when a subtree needs bespoke guidance; keep the
   text short, directive, and linked back to canonical docs.
3. Run `make agents` to confirm the repository-wide policy check still passes.

Following these steps keeps instructions discoverable and ensures reviewers can
trust the documented expectations.

## Pull Requests

- Use conventional commits where practical (e.g., `feat:`, `fix:`, `docs:`).
- Confirm `make lint`, `make test`, and `make coverage MIN_COVERAGE=95` pass
  locally (docs-only changes can note "not applicable" in the template).
- Reference any integration or bench commands you ran when touching those
  surfaces.
- Keep pull request descriptions clear: outline the behaviour change, the tests
  you ran, and any follow-ups required.

Thanks for keeping OCI CPU Shaper healthy!
