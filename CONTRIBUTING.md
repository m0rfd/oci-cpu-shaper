# Contributing to OCI CPU Shaper

Thank you for helping improve OCI CPU Shaper. The sections below mirror the `docs/08-development.md` workflow so every contribution includes the metadata and verification signals required by §8.7.

## Filing issues

1. **Use the templates.** Select one of the GitHub issue templates (`Bug report`, `Feature request`, or `Docs feedback`) under `.github/ISSUE_TEMPLATE/`. Each template captures OCI tenancy context, environment details, reproduction commands, and checkboxes for the `make lint`, `make test`, and `make coverage MIN_COVERAGE=95` expectations from `docs/08-development.md` §§11 & 14. Providing this data keeps triage focused on the failing surfaces instead of chasing missing configuration details.
2. **Link supporting material.** Include log excerpts, OCI compartment/tenancy OCIDs, and screenshots whenever applicable. When a checkbox is not applicable (for example, docs-only feedback), note why so maintainers can still follow the §8.7 triage workflow.
3. **Review the roadmap.** If you are opening a feature request, check `docs/ROADMAP.md` to avoid duplicate proposals and to reference the milestone your idea fits into.

## Pull requests

1. **Follow the development workflow.** Run `make lint`, `make test`, and `make coverage MIN_COVERAGE=95` (or `make check` plus the coverage target) locally before submitting changes. These commands already configure the caches referenced in `docs/08-development.md`, keeping results consistent across environments.
2. **Update documentation.** When behaviour or configuration changes, edit the relevant files under `docs/` (and `docs/CHANGELOG.md`) so operators understand the new expectations.
3. **Reference issues.** Link the issue you are addressing in the pull request description, summarise the verification done, and mention any follow-up work that should be tracked separately.

## Maintainer triage checklist

Review new issues against the `docs/08-development.md §8.7` workflow:

1. Acknowledge the report, apply the `triage` label, and verify that the author provided the required environment and OCI tenancy metadata from the template.
2. Classify the issue using the established area/severity labels (`bug`, `enhancement`, `documentation`, `controller`, etc.).
3. Reproduce the behaviour (or confirm the feature request) using the supplied commands, `make` targets, or OCI tenancy context. Request missing data—especially coverage/test evidence—before escalating.
4. Decide whether the item feeds into a hotfix, the roadmap backlog, or can be closed out of scope. Capture follow-up actions inside the issue thread and keep `docs/CHANGELOG.md` updated when fixes land.

Consistently applying the templates plus §8.7 keeps maintainers aligned on priorities, ensures new contributors receive fast feedback, and guarantees each report carries the OCI tenancy information required to recreate the scenario locally.
