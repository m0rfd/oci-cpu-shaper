# AGENTS

## Scope: `pkg/shape/`
- Exported pool APIs (`NewPool`, `SetPauseThresholds`, etc.) live in `pool.go` with corresponding `pool_*_test.go` suites; keep benchmarks separate in `pool_benchmark_test.go`.
- Worker scheduling and the optional rootful `trySchedIdle` hook belong in `worker.go` and `pool_rootful.go`; avoid mixing pause logic or busy-wait helpers into these files.
- Pause/resume thresholds stay in `pause.go`, and duty-cycle helpers that manage tight loops remain in `busywait.go`; mirror every helper with dedicated tests to maintain the ≥95% coverage target from the root AGENT.
- Share fixtures through `testhelpers_test.go` and keep build-tagged behavior isolated so rootless builds remain unaffected.
- Follow the repo QA workflow referenced in the root AGENT by rerunning `make lint`, `make test`, and `make coverage MIN_COVERAGE=95` after changes and documenting any new tunables.
