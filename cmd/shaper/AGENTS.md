# AGENTS

## Scope: `cmd/shaper/`
- Keep the CLI entrypoint layered: `entrypoint.go` only builds `runDeps`, `run_deps.go` holds exit codes/wiring helpers, `logging_metadata.go` owns logger + IMDS resolution, `metrics_http.go` owns HTTP/metrics wiring, and `cgroup_inspection.go` contains cgroup probing. `app.go`/`app_run.go` handle signal-safe orchestration, and `controller_helpers.go` wires the shaper to `pkg/adapt` without embedding controller logic here.
- Define or change flags/env bindings exclusively in `options.go` (and `options_test.go`) so every switch has unit coverage and stays aligned with the reference tables in `docs/09-cli.md`.
- Runtime configuration structs and helpers now live in `pkg/runtimeconfig`; keep this layer focused on wiring/IO and push config changes (defaults, loading, env overlays, validation, and conversions) into that package with matching unit coverage.
- Wiring helpers (`metrics_client_*.go`, `run_deps_*.go`, `recorder_logger.go`, `metrics.go`) should stay focused on dependency injection and logging; move behavioral changes into `pkg/` packages instead of growing the CLI layer.
- Per the repository root AGENT, rerun `make lint`, `make test`, and `make coverage MIN_COVERAGE=95` after editing this tree, and update the relevant docs whenever the CLI surface shifts.
