# AGENTS

## Scope: `pkg/oci/`
- `client.go` owns exported constructors plus Instance Principal wiring; keep interface-oriented seams so tests can swap clients without build tags. `ClientFactory` now drives those seams—prefer `WithFactory` overrides (and document new knobs) instead of mutating package-level globals.
- Build or adjust Monitoring queries inside `query.go`, keeping the `CpuUtilization[1m]{...}.percentile(0.95)` helpers separate from pagination/aggregation glue and covering changes in `query_test.go`. Reuse scoped fixtures instead of piling helpers back into `query_test.go`: HTTP mocks live in `monitoring_http_test.go`, SDK helper fixtures plus fake providers sit in `sdk_overrides_test.go`, constructor/factory cases belong in their focused suites, and pagination helpers stay colocated with the query logic.
- OCI SDK shims stay in `sdk_client.go`; avoid importing the SDK elsewhere in this package, and keep request/response translation here.
- Offline/static fixtures for tests belong in `static.go` (and helpers in `monitoring.go`); do not bake sample data into production functions.
- Per the repo-wide QA rules (root AGENT), rerun `make lint`, `make test`, and `make coverage MIN_COVERAGE=95` before sending patches and refresh docs when API contracts shift.
