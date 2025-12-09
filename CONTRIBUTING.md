# Contributing to OCI CPU Shaper

OCI CPU Shaper ships deterministic tooling so local changes line up with CI. The
notes below expand on `README.md` and [`docs/08-development.md`](docs/08-development.md),
incorporate the §8.7 triage workflow, and should be reviewed before opening an
issue or pull request.

## Getting Started

1. Discuss substantial changes in an issue before opening a pull request so the
   architecture remains aligned with `docs/initial-implementation-plan.md`.
2. Install the pinned toolchain (Go 1.25.5, `golangci-lint` 2.6.1)
   with `make tools` or `mise install` as described in
   §14 of `docs/08-development.md`.
4. Configure Git hooks via `git config core.hooksPath .githooks` if you want the
   optional pre-push checks that run `make lint` for you.

## Tooling Workflow

Run the following helpers from the repository root before every pull request:

- `make lint-fix` to execute `golangci-lint` with auto-fix enabled and format the Makefile.
- `make lint` to run Go checks without modifying files.
- `make lint-makefile` to validate and check Makefile formatting.
- `make test` to run `go test -race ./...` so race conditions surface early.
- `make coverage MIN_COVERAGE=96` to regenerate `coverage.out`/`coverage.txt` and
  prove repository-wide statement coverage stays at or above the required 96 %.
- `make codeql-setup` to install the CodeQL CLI into a versioned toolcache and
  prefetch the default query packs into `.cache/codeql/packs` for offline runs.
- `make codeql`, `make codeql-actions`, `make codeql-go`, or `make codeql-all`
  to mirror the PR CodeQL checks. Run `CHECK_INCLUDE_CODEQL=0 make check` when
  you want to match CI’s faster path without CodeQL; the default bundles CodeQL
  into the broader validation locally. These targets store databases under
  `.cache/codeql/databases`, honor `CODEQL_DATABASE_ROOT`/`CODEQL_PACK_CACHE_DIR`
  overrides, emit SARIF files to `artifacts/codeql/`, and rely on the baked-in
  Go build command so no manual build steps are required. Override the query
  packs with `CODEQL_ACTIONS_QUERY_PACK` or `CODEQL_GO_QUERY_PACK` when testing
  variants, and use `make codeql-clean` to drop generated databases and SARIF
  outputs without touching the cached CLI or packs.

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

## Filing issues

Thank you for helping improve OCI CPU Shaper. The sections below mirror the
`docs/08-development.md` workflow so every contribution includes the metadata
and verification signals required by §8.7.

1. **Use the templates.** Select one of the GitHub issue templates (`Bug report`,
   `Feature request`, or `Docs feedback`) under `.github/ISSUE_TEMPLATE/`. Each
   template captures OCI tenancy context, environment details, reproduction
   commands, and checkboxes for the `make lint`, `make test`, and
   `make coverage MIN_COVERAGE=96` expectations from `docs/08-development.md`
   §§11 & 14. Providing this data keeps triage focused on the failing surfaces
   instead of chasing missing configuration details.
2. **Link supporting material.** Include log excerpts, OCI compartment/tenancy
   OCIDs, and screenshots whenever applicable. When a checkbox is not applicable
   (for example, docs-only feedback), note why so maintainers can still follow
   the §8.7 triage workflow.
3. **Review the roadmap.** If you are opening a feature request, check
   `docs/ROADMAP.md` to avoid duplicate proposals and to reference the milestone
   your idea fits into.

## Pull Requests

- Use conventional commits where practical (e.g., `feat:`, `fix:`, `docs:`) and
  keep the description clear: outline the behaviour change, verification, and
  any follow-ups required.
- **Follow the development workflow.** Run `make lint`, `make test`, and
  `make coverage MIN_COVERAGE=96` (or `make check` plus the coverage target)
  locally before submitting changes. These commands already configure the caches
  referenced in `docs/08-development.md`, keeping results consistent across
  environments.
- **Update documentation.** When behaviour or configuration changes, edit the
  relevant files under `docs/` (plus `docs/CHANGELOG.md`) so operators
  understand the new expectations.
- **Reference issues.** Link the issue you are addressing in the pull request
  description, summarise the verification done (including integration/bench
  targets when applicable), and mention any follow-up work that should be tracked
  separately.

Thanks for keeping OCI CPU Shaper healthy!

## Maintainer triage checklist

Review new issues against the `docs/08-development.md §8.7` workflow:

1. Acknowledge the report, apply the `triage` label, and verify that the author
   provided the required environment and OCI tenancy metadata from the template.
2. Classify the issue using the established area/severity labels (`bug`,
   `enhancement`, `documentation`, `controller`, etc.).
3. Reproduce the behaviour (or confirm the feature request) using the supplied
   commands, `make` targets, or OCI tenancy context. Request missing data—
   especially coverage/test evidence—before escalating.
4. Decide whether the item feeds into a hotfix, the roadmap backlog, or can be
   closed out of scope. Capture follow-up actions inside the issue thread and
   keep `docs/CHANGELOG.md` updated when fixes land.

Consistently applying the templates plus §8.7 keeps maintainers aligned on
priorities, ensures new contributors receive fast feedback, and guarantees each
report carries the OCI tenancy information required to recreate the scenario
locally.
