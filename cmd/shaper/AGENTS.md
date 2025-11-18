# AGENTS

## Scope: `cmd/shaper/`
- Keep the CLI entrypoint layered: `main.go` should only build `runDeps`, `app.go`/`app_run.go` handle signal-safe orchestration, and `controller_helpers.go` wires the shaper to `pkg/adapt` without embedding controller logic here.
- Define or change flags/env bindings exclusively in `options.go` (and `options_test.go`) so every switch has unit coverage and stays aligned with the reference tables in `docs/09-cli.md`.
- Configuration helpers are intentionally sliced by concern: defaults (`config_defaults.go`), file loading (`config_loader.go`), merges (`config_merge.go`), env overlays (`config_env.go`), type definitions (`config_types.go`), and validation (`config_validate.go`). Keep helpers in their respective files and mirror changes in the corresponding `*_test.go` suites.
- Wiring helpers (`metrics_client_*.go`, `run_deps_*.go`, `recorder_logger.go`, `metrics.go`) should stay focused on dependency injection and logging; move behavioral changes into `pkg/` packages instead of growing the CLI layer.
- Per the repository root AGENT, rerun `make lint`, `make test`, and `make coverage MIN_COVERAGE=95` after editing this tree, and update the relevant docs whenever the CLI surface shifts.
