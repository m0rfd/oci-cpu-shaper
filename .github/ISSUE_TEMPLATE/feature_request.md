---
name: "Feature request"
about: "Propose a new capability or workflow improvement"
title: "[Feature]: "
labels:
  - enhancement
  - triage
---

## Summary

Describe the user story and the outcome this feature should enable.

## OCI tenancy / consumer context

Detail the tenancy OCID(s), compartments, or deployment tiers (Always Free, paid, self-hosted) that need this feature.

## Environment

- **Regions / shapes involved:**
- **Current release or commit:**
- **Interfaces exercised (CLI, Compose, Quadlet, SDK, etc.):**

## Proposed change

Outline the expected behaviour, API/config updates, and any docs/UI impact.

## Reproduction or validation commands

Share the commands, configuration files, or runbooks you expect to use to validate the new capability.

```bash
# example commands / PoC snippets
```

## Verification (docs/08-development.md §§11 & 14)

- [ ] `make lint`
- [ ] `make test`
- [ ] `make coverage MIN_COVERAGE=95`

If you have not exercised a checkbox yet, state the planned validation so maintainers can triage per §8.7.

## Additional context

Link related issues, roadmap items, or OCI services touched by the proposal.
