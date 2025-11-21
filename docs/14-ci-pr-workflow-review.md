# §§14 – Pull Request CI Review

## Observations
- The CI workflow now adds workflow-level concurrency, a dedicated formatting gate, and unified caching over the repository-scoped `.cache/` directories so repeated jobs reuse Go build products.
- Linting, vulnerability scanning, policy validation, unit tests, coverage, and integration runs all delegate to the Makefile helpers, ensuring contributors and CI share the same toolchain bootstrap (including cache warming for golangci-lint and govulncheck).
- A distinct `make test` job complements the race-enabled coverage target, while the coverage job now posts its summary to the PR via the step summary for fast reviewer feedback on the ≥96% requirement.
- Integration runs now reuse the shared Go cache and rely on `make integration`, preserving the generated log artifacts for PR diagnostics when `INTEGRATION_KEEP_LOGS` is set.

## Follow-ups
- Monitor whether the container smoke job requires similar Makefile wrapping to centralize Docker build logic; if future changes introduce host-side Go tooling there, mirror the caching pattern established for the other jobs so rebuilds continue to benefit from `.cache/go` reuse.
