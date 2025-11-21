# AGENTS

## Scope: `cmd/`
- `cmd/shaper` stays a thin wiring layer with the `app` wiring struct handling the run loop, signal plumbing, controller boot, and metrics setup. Keep these coordination files thin and push logic into `pkg/` (plan §§5.1, 15).
- Flags/env must mirror §5.2; document new options in `docs/` as part of each change. Flag definitions and normalization logic now live in `cmd/shaper/options.go`, so extend that file (and its unit tests) when adding new CLI switches instead of bloating `main.go`.
- Preserve friendly logging/exit codes and pair CLI tweaks with smoke/unit coverage (§11.1).
- Hold coverage ≥99% (`make coverage`) and add tests for every new flag path before merging.
- Keep shaper tests scoped by concern—mirror the production layout (options, metrics server, controller boot, etc.) and prefer focused files over monolithic suites. Split new suites before they exceed ~300 lines so reviewers can reason about failures quickly.
- Shared shaper test helpers now live in `cmd/shaper/testhelpers_controller_test.go`, `cmd/shaper/testhelpers_metrics_test.go`, `cmd/shaper/testhelpers_http_test.go`, and `cmd/shaper/testhelpers_logging_test.go`; keep helpers aligned with those scopes.
- Deduplicate cross-suite stubs/fakes in those helper files and move bespoke helpers next to the suites that use them instead of bloating shared fixtures.
- Config wiring is now split across focused files: `config_defaults.go`, `config_loader.go`, `config_merge.go`, `config_env.go`, and `config_validate.go`. Keep each concern scoped (e.g., do not add validation helpers to the env file) and mirror the structure with `config_*_test.go` coverage when new helpers appear.
