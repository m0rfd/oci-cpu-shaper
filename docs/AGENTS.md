# AGENTS

## Scope: `docs/`
- Mirror current behavior: revise affected docs and `docs/CHANGELOG.md` per §12 whenever features/configs shift.
- When documenting defaults, cross-check `pkg/adapt/config_defaults.go` and `pkg/runtimeconfig/defaults.go`, ensuring tables/examples mirror the authoritative values there (current deployed band: start 0.22, min/max 0.20/0.32, suppression 0.80/0.68, runnable 1.20/0.96, smoothing 5) and align with `configs/*.yaml` plus `docs/09-cli.md`/`docs/initial-implementation-plan.md`.
- Capture QA rules: mention ≥98% coverage (`make coverage`) and the required test updates when you describe workflows.
- Keep headings tight and aligned with plan section numbers (e.g., §§4–7) for quick cross-reference.
- Preserve OCI/kernel citations; add new references using Markdown reference links.
- Keep reclaim guardrail language (22–27% P95 band over the 20% floor) aligned across `docs/initial-implementation-plan.md`,
  `docs/03-free-tier-reclaim.md`, `docs/05-execution-flow.txt`, `docs/07-alarms.md`, and related guides.
- For CLI wiring or architecture notes, align edits with `docs/05-execution-flow.txt` §3.1 and refresh that section when scopes move; keep §0 diagrams current with runtime config and controller layers.
- Treat `pkg/runtimeconfig/defaults.go` and `pkg/adapt/config_defaults.go` as the sources of truth for configuration defaults and keep `docs/initial-implementation-plan.md` synchronized with any changes.
