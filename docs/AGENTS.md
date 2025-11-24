# AGENTS

## Scope: `docs/`
- Mirror current behavior: revise affected docs and `docs/CHANGELOG.md` per §12 whenever features/configs shift.
- When documenting defaults, cross-check `pkg/adapt/config_defaults.go` and ensure tables/examples mirror the authoritative valu
es there.
- Capture QA rules: mention ≥96% coverage (`make coverage`) and the required test updates when you describe workflows.
- Keep headings tight and aligned with plan section numbers (e.g., §§4–7) for quick cross-reference.
- Preserve OCI/kernel citations; add new references using Markdown reference links.
- For CLI wiring or architecture notes, align edits with `docs/05-execution-flow.txt` §3.1 and refresh that section when scopes move; keep §0 diagrams current with runtime config and controller layers.
