---
name: "Docs feedback"
about: "Report gaps or improvements for the documentation set"
title: "[Docs]: "
labels:
  - documentation
  - triage
---

## Summary

Describe the inaccurate, missing, or outdated guidance.

## Location

List the exact path(s) and section heading(s) (for example, `docs/08-development.md §8.7`).

## OCI tenancy / environment context

Share the tenancy OCID(s), compartments, regions, or deployment surfaces where the documentation diverges from reality.

## Reproduction commands or screenshots

Provide the CLI/Compose commands, `make` targets, or UI steps that surfaced the problem.

```bash
# example commands
```

## Verification (docs/08-development.md §§11 & 14)

- [ ] `make lint`
- [ ] `make test`
- [ ] `make coverage MIN_COVERAGE=99`

For doc-only issues you can mark the boxes as not applicable, but please clarify why so the §8.7 triage checklist remains consistent.

## Suggested fix

Share replacement text, diagrams, or links to authoritative OCI references.
