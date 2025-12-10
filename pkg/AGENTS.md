# AGENTS

## Scope: `pkg/`
- Honor package seams from plan §§5.1 & 15; keep exported APIs tight with GoDoc comments.
- Follow §§3, 5, 9 for retries, fallbacks, and controllers; pair logic edits with focused unit tests (§11.1).
- Maintain ≥98% coverage via `make coverage`; expand suites before merging when new paths appear.
- Avoid busy-waiting—workers must respect the §10 duty-cycle budget.
- `pkg/shape` keeps exported pool APIs in `pool.go`; extend worker loops in `worker.go`,
  pause/resume rules in `pause.go`, and busy-wait helpers in `busywait.go` to avoid
  tangling rootful build tags with general logic.
- `pkg/oci` splits responsibilities across files: constructors, global wiring, and
  Instance Principal helpers live in `client.go`; query/aggregation utilities (e.g.,
  `QueryP95CPU`, pagination folding) remain in `query.go`; the OCI SDK adapter plus
  HTTP glue (request building, response decoding) stay in `sdk_client.go`. Extend OCI
  clients by implementing the `metricsClient` interface and passing it to `newClient`
  (or `newTestClient`) so tests can continue mocking calls without build-tag changes.
- `pkg/imds` follows a three-file split: constructor/config wiring belongs in
  `client_config.go`, metadata operations live in `operations.go`, and HTTP/retry
  helpers (including new IMDS endpoints) stay in `transport.go`. Keep the retry
  budget/backoff expectations in `transport.go` aligned with §3/§5 of the plan and
  mirror that structure in the `*_test.go` suites when adding endpoints or knobs.
- `pkg/runtimeconfig` owns the shared runtime configuration structs plus the defaults,
  file loader, env overlays, adapt translation helpers, and validators used by
  `cmd/shaper`. Push new config surface area here and extend the colocated tests
  so future binaries can reuse the same flow.
- Keep unit tests colocated with the component they cover (`pool_api_test.go`, `pause_test.go`,
  `worker_test.go`, etc.) and share fixtures via `testhelpers_test.go`. Prefer exercising exported APIs,
  only falling back to `//nolint:testpackage` access when a worker hook or probe must be overridden.
  Avoid sprawling integration-style cases outside the dedicated load/build tagged suites.
