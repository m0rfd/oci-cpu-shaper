# AGENTS

## Scope: `docs/`
- Mirror current behavior: revise affected docs and `docs/CHANGELOG.md` per §12 whenever features/configs shift.
- Capture QA rules: mention ≥96% coverage (`make coverage`) and the required test updates when you describe workflows.
- Keep headings tight and aligned with plan section numbers (e.g., §§4–7) for quick cross-reference.
- Preserve OCI/kernel citations; add new references using Markdown reference links.
- For CLI wiring or architecture notes, align edits with `docs/05-execution-flow.txt` §3.1 and refresh that section when scopes move; keep §0 diagrams current with runtime config and controller layers.
- Treat `pkg/runtimeconfig` and `pkg/adapt` as the sources of truth for configuration defaults and keep `docs/initial-implementation-plan.md` synchronized with any changes.
