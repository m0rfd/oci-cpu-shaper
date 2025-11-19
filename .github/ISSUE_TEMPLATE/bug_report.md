---
name: "Bug report"
about: "Report a regression or defect in OCI CPU Shaper"
title: "[Bug]: "
labels:
  - bug
  - triage
---

## Summary

Describe the unexpected behaviour and the impact to OCI CPU Shaper users.

## Environment

- **OCI region(s):**
- **Compartment / tenancy OCID(s):**
- **Compute shape / OS image:**
- **Go toolchain (`go version`):**
- **Deployment mode (CLI, Compose, Quadlet, runner, etc.):**

## Reproduction Steps

List every command and configuration value required to reproduce the issue locally. Include the exact `make` targets (for example, `make check`) and any OCI tenancy configuration that influenced the result.

```bash
# sample reproduction commands
```

## Observed output / logs

Attach console logs, stack traces, or metrics that demonstrate the failure.

## Verification (docs/08-development.md §§11 & 14)

- [ ] `make lint`
- [ ] `make test`
- [ ] `make coverage MIN_COVERAGE=95`

If a checkbox does not apply, explain why so maintainers can align the issue with the §8.7 triage workflow.

## Additional context

Share mitigation steps, regression windows, or related issues/PRs.
