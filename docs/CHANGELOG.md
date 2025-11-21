# Changelog

## Unreleased

### Added
_Note coverage-impacting additions: mention new test suites or tooling that shift the CI ≥95% statement coverage budget (§11)._
- Execution-flow reference (`docs/05-execution-flow.txt`) summarising the CLI,
  adaptive controller loops, and supporting packages with a text diagram so
  operators can trace the §3 runtime wiring without scanning the codebase (§§3,
  5, 12).
- Configurable host-load pause/resume thresholds for the worker pool plus CLI YAML and
  environment knobs so estimator observations suspend the pool until the host cools.
  `pkg/shape` now exposes pause state helpers/tests, the controller forwards host CPU
  readings to the pool, and docs (§§3.1, 9) describe the new configuration and
  behaviour. Updated tests cover pause transitions to keep the ≥95% coverage floor (§§3,
  5, 9, 11).
- Cgroup telemetry helper that reads `/proc/self/cgroup`, parses the
  colocated `cpu.weight`/`cpu.max` files, and publishes the detected values via
  new `cgroup_cpu_weight`/`cgroup_cpu_max_*` metrics plus a `cgroup` block in
  `/healthz`. Startup logs now warn whenever the observed weight exceeds the
  low-weight defaults documented in §4 so Compose/Quadlet drift is obvious.
  Fresh unit tests cover the helper, metrics exporter, and `/healthz` handler to
  keep the ≥95% coverage floor intact, and §§4, 9 describe the new telemetry
  surfaces for operators.
- Grafana dashboard export (`deploy/grafana/oci-cpu-shaper-dashboard.json`) covering OCI
  P95, controller target/state, and host CPU overlays, plus §5.4 import instructions so
  operators can wire the Prometheus feed into Grafana without rebuilding the charts (§§3,
  5, 12).
- Controller metrics now export `controller_interval_seconds` and
  `controller_last_error_info`, with adaptive-controller hooks that update the interval,
  state, and last OCI error every loop. Tests cover the Prometheus output and the
  fallback integration path, while §5.4 now recommends Grafana panels for the cadence
  and error strings (§§5, 9, 11).
- `/healthz` status handler on the metrics listener that surfaces controller
  state plus the last OCI Monitoring and estimator errors as JSON; unit tests
  cover `pkg/http/status` and the offline CLI E2E now exercises the endpoint to
  keep the ≥95% statement coverage floor intact (§§5, 9, 11). The handler now
  also reports the controller mode, and companion tests verify the `/metrics`
  export plus HTTP bind failures so the Prometheus listener fails fast when the
  port is unavailable (§§3.2, 4, 5, 9, 11).
- `shaper --version`/`shaper version` commands that print the embedded build
  metadata without initialising configuration or logging, plus unit coverage to
  ensure the fast-exit path leaves existing logger wiring untouched (§§5, 9).
- Configuration samples `configs/mode-a.yaml`/`configs/mode-b.yaml` that ship the
  documented goal band, suppression thresholds, and HTTP bind defaults, plus CLI
  tests to ensure they load without overrides. §9 now links the manifests and
  explains how environment variables layer on top for Mode A/Mode B deployments.
- CLI emulation suite under `tests/e2e/` gated by the `e2e` build tag, complete with reusable IMDS/Monitoring mocks and `make e2e` helper so offline/online controller flows, metrics output, and structured state-transition logs stay verifiable in CI and locally (§§5, 9, 11).
- Rootful worker pools compiled with `-tags rootful` now request Linux
  `SCHED_IDLE` via `sched_setscheduler(0, SCHED_IDLE, ...)` as the pool is
  constructed, record the result until the CLI installs its warning handler, and
  emit a `worker failed to enter sched_idle` warning when kernels reject the
  downgrade (for example when `CAP_SYS_NICE`/`SYS_NICE` is missing). Unit tests
  swap in fake schedulers to exercise success, permission-denied, and error
  propagation paths, preserving the §11.1 coverage contract while documenting
  the new behaviour in §§6 and 9.
- Integration test `TestSchedIdleWarningTracksSysNiceCapability` builds the
  rootful binary with `-tags rootful`, launches the Mode B container once as the
  `nobody` user (missing capabilities) and once as `root` with `SYS_NICE`
  explicitly granted, and asserts the sched_idle warning only appears when the
  capability is missing. README §10 now references the workflow so operators can
  validate hosts before enabling SCHED_IDLE (§§6, 10, 11).
- Regression suite `TestControllerCpuUtilisationAcrossOCPUs` covering 1–4 OCPU CpuUtilization streams and the relaxed-interval clamp so the adaptive controller keeps the Always Free reclaim guardrails documented in §§3.1 and 5.2. Tests maintain the ≥95% statement floor by exercising the prolonged high-utilisation path in `pkg/adapt/controller.go` (§11).
- Deterministic 24-hour-equivalent worker-pool load harness (`go test -tags=load ./pkg/shape -run TestPoolLoad24hEquivalent`) that logs CPU/RSS telemetry to `artifacts/load/pool-24h.log` and enforces the §10 budgets alongside nightly/manual CI coverage via `.github/workflows/load.yml` (§§10, 11.4).
- Duty-cycle benchmark suite (`BenchmarkPoolDutyCycle`) plus the `hack/check_benchmarks.sh` guard script that record CPU usage, per-tick drift, and scheduler fairness across multiple quantums and targets, failing whenever the §10 duty-cycle or §5 scheduler limits regress (§§5, 10, 11).
- Always Free Terraform stack under `deploy/terraform/self-hosted-runner/` that provisions a hardened GitHub Actions runner with instance-principal access scoped to test compartments, including cloud-init hardening and IAM automation (§§5, 8, 15).
- Terraform alarm module under `deploy/terraform/alarms/` that creates the seven-day P95 Always Free reclaim guardrail with parameterised OCIDs, opinionated tagging, and Notification wiring, plus documentation of the exact `.window(7d).percentile(0.95)` MQL expression in §7 (§§3, 5, 7).
- Scheduled `self-hosted` workflow exercising IMDS lookups, live `QueryP95CPU` calls via `hack/tools/p95query`, Docker cgroup v2 behaviour, and a rootful CPU weight validation that builds the image in situ, runs the high/low weight hog containers, and publishes `/sys/fs/cgroup` logs as artifacts on the OCI runner (§§4, 6, 11, 15).
- Guardrail verification CLI (`hack/tools/alarmguard`) wired into the `self-hosted` workflow so instance-principal checks fail CI whenever the Always Free P95 alarm is missing, disabled, or misconfigured (§§5, 7, 11, 15).
- Runner maintenance and secrets rotation guidance in §15 of `docs/08-development.md`, covering patch cadence, token refresh, and repository variables linked to the new workflow (§§8, 12, 15).
- Dependabot automation covering Go modules, GitHub Actions, and container Dockerfiles with weekly/monthly cadences to keep CI and release dependencies current (§§11, 14).
- Documented the §8.7 issue triage workflow so contributors can acknowledge, classify, and reproduce reports consistently across tooling and coverage expectations (§§8, 11, 12, 15).
- Adaptive controller wiring from `cmd/shaper` to the OCI Monitoring client, estimator sampler, and worker pool, plus layered YAML + environment configuration for controller targets, cadences, worker counts, and HTTP binding (§§3.1, 5.2). Tests cover configuration decoding, environment overrides, and controller factory success/error paths to preserve the ≥95% coverage floor (§11).
- Fast-loop suppression mode that adds a `suppressed` controller state, host-load hysteresis, and configuration knobs (`controller.suppressThreshold`/`controller.suppressResume`, `SHAPER_SUPPRESS_THRESHOLD`/`SHAPER_SUPPRESS_RESUME`) so the estimator can drop the worker pool to zero until the host cools (§§3.1, 5.2). Unit tests now cover suppression entry/exit and estimator error recording while docs in §§4 and 9 describe the new telemetry and structured `controllerState` logging.
- Instance-principal Monitoring client (`pkg/oci`) exposing `QueryP95CPU` with pagination, missing-data fallbacks, and HTTP-backed mocks that keep coverage above the ≥95% floor. Documented in §5 alongside troubleshooting guidance for tenancy policy and metric gaps.
- HTTP-backed IMDSv2 client with retried metadata lookups, shapeConfig decoding, and an overridable endpoint (`OCI_CPU_SHAPER_IMDS_ENDPOINT`), documented in §2 and backed by `httptest` unit coverage (§§2, 5, 11).
- Repository-wide AGENTS policy check with `make agents` and CI coverage to enforce scoped instructions (§8.4).
- Token-optimised AGENTS templates and directory-change checklist to keep scoped guidance current (§8.6).
- Distroless Docker targets, Compose manifests, and runtime scripts for Komodo Mode A (§6).
- Rootful Mode B Compose manifest, Quadlet unit, and documentation covering host capability requirements (§6).
- Documented bootstrap CLI flags, configuration layout, and diagnostics in §§5 and 9 references.
- Time-bounded shutdown support via the `--shutdown-after` flag so smoke tests and diagnostics can exercise the adaptive controller without leaving background processes behind; docs cover the workflow alongside the offline configuration shipped in the image (§§5, 9).
- GitHub Actions workflows covering `golangci-lint` and race-enabled `go test` runs on pull requests (§14).
- Automated release pipeline publishing multi-architecture images with Syft-generated SPDX SBOM artifacts (§14).
- Release workflow now installs Cosign, signs each multi-arch image digest with GitHub Actions OIDC keyless certificates, emits SPDX attestations, and uploads the detached signatures/certificates as release assets so operators can verify images offline (§14).
- Unit coverage for IMDS dummy metadata, controller mode wiring, and CLI bootstrap flows via dependency-injected smoke tests (§§5, 9, 11).
- Race-enabled `make coverage` target and CI enforcement requiring at least 95% statement coverage before merging (§14).
- Go vulnerability scanning via `make govulncheck` and a dedicated CI job that restores module/build caches, failing pull requests when published advisories affect the dependency graph (§14).
- CPU weight responsiveness integration suite with CI coverage on `ubuntu-latest` (cgroup v2) that exercises the container build alongside a competing workload and publishes verbose logs (§§6, 11).
- Local `make integration` helper replicating the CI cgroup v2 guard, Docker availability checks, and log capture so contributors can rerun the CPU weight suite with artifact parity (§§6, 11).
- CPU weight responsiveness harness now builds and runs both the distroless rootful and rootless images, gating execution on the cgroup v2 cpu controller so CI and local runs validate both deployment paths (§§6, 8, 11).
- Quick Start onboarding guide that condenses the five plan-mandated console steps and links to the IAM, Monitoring, Compose, and alarm references (§10).
- Documentation refresh covering OCI IAM policy setup (§1), Always Free reclaim guardrails (§3), cgroup v2 tuning guidance (§4), and alarm workflows (§7), aligning `docs/` with the implementation plan’s required artifacts (§12).
- `/metrics` exporter and Prometheus integration surfaced through the CLI, including emitted series, sample scrape output, and Compose/HTTP_ADDR wiring documented across §§4–9.

### Changed
_Record coverage reductions or mitigations so reviewers can audit the CI ≥95% threshold impact (§11)._
- Refreshed `docs/05-execution-flow.txt` to document the CLI entrypoint file
  boundaries and responsibilities, and linked `docs/AGENTS.md` plus
  `cmd/shaper/AGENTS.md` back to the architecture section so wiring guidance
  stays current (§§3, 8, 12).
- Raised the Go toolchain to 1.25.4 in `go.mod`, `.tool-versions`, and the container build ARG so CI, local builds, and release
  images track the latest stable release, and refreshed badges/docs to match (§§8, 12, 14).
- CLI documentation (§9), `runtimeconfig.Default()`, and `adapt.DefaultConfig()` now
  match the Mode A/Mode B manifests exactly (0.22 target start, 0.20–0.32 band, 4 hour
  relaxed interval, 2 second estimator loop, 0.80/0.68 suppression + pool thresholds,
  two-worker pool, etc.), keeping the published YAML and binary defaults aligned.
- Metadata logging now queries IMDS for canonical-region names even when
  `OCI_REGION` overrides are provided, only falling back to the override when
  IMDS is unavailable so startup logs and `/healthz` continue to report the
  true canonical region (§§2, 9).
- The CI golangci-lint job now fails when `.golangci.yml`'s auto-fixable
  formatters (gci, gofmt, gofumpt, goimports, golines, swaggo) mutate files
  during `make lint`; the workflow prints the diff and reminds contributors to
  commit the fixes instead of allowing silent changes to slip through (§§8, 11,
  14).
- Controller configuration now accepts `controller.suppressThreshold=0` (or
  `SHAPER_SUPPRESS_THRESHOLD=0`) to disable host-load suppression entirely. The
  resume threshold is ignored when disabled, normalization preserves the zero
  values, and validation skips the previous `target*` comparisons so operators
  can opt out of estimator-driven shutdowns without tripping the CLI safety
  rails (§§3.1, 5.2, 9, 11).
- `HTTP_ADDR` environment overrides now accept an empty string to disable the
  `/metrics` listener even when the YAML manifest specifies a bind address.
  Setting `HTTP_ADDR=` helps smoke tests and container diagnostics avoid
  exposing the endpoint while still recording metrics internally (§§6, 9).
- `pkg/oci` constructors now accept a `ClientFactory` via the new `WithFactory(...)` option so tests and the CLI swap Monitoring
  mocks without mutating package-level globals. `cmd/shaper` wires the factory into the production constructor and §5 documents
  the seam, keeping the existing ≥95% coverage floor intact by exercising the new paths in the unit suites.
- CLI environment variable defaults now document the positive
  `SHAPER_STEP_UP`/`SHAPER_STEP_DOWN` values enforced in code and explain that
  `StepDown` stays positive because the controller subtracts it internally
  (§§3.1, 5.2, 9).
- Runtime configuration loader now validates target/goal bounds, positive controller/estimator intervals, worker counts, and step
  sizes after layering YAML files with environment overrides, returning `adapt.ErrInvalidConfig` when misconfigured values are
  detected. Fresh CLI unit tests cover invalid manifests and environment overrides so the ≥95% coverage target remains intact
  (§§3.1, 5.2, 9, 11).
- Runtime configuration structs, defaults, YAML/env overlays, validation, and the
  controller translation helper moved from `cmd/shaper` into the shared
  `pkg/runtimeconfig` package. The CLI now imports this package instead of owning
  the helpers, `docs/09-cli.md` explains the shared flow, and tests migrated with
  the code so future binaries can consume the same API (§§3.1, 5.2, 9, 11).
- Distroless image builds now reference the repository-root `Dockerfile` and ship the
  offline smoke-test config from `configs/offline-smoke.yaml`, consolidating Komodo,
  CI, and release workflows on a single path (§6).
- Alarm documentation, Terraform module defaults, and the `alarmguard` verification now reflect OCI Monitoring’s one-day CpuUtilization interval limit (the console ignores `.window()`), document the split-by-`resourceId` workflow for multi-runner alarms, and keep the self-hosted guardrail enforcement aligned with what operators can configure (§§5, 7, 11).
- Alarm documentation, Terraform module defaults, workflow wiring, and the `alarmguard` verifier now pin CpuUtilization alarms to `CpuUtilization[1d]{resourceId="<ocid>"}.percentile(0.95) < 20` so both the docs and CI enforcement match OCI’s maximum one-day interval (§§5, 7, 11).
- The distroless targets now copy the Mode A/Mode B manifests into
  `/etc/oci-cpu-shaper/configs/` and the Compose/Quadlet/docker-run helpers default
  to those paths, letting deployments start with baked-in YAML while retaining the
  option to bind-mount overrides (§§6, 8).
- Runtime metadata resolution now prefers IMDS compartment/region lookups and only
  falls back to YAML/environment overrides when the platform APIs fail or when
  `oci.offline` is set. Updated unit tests cover the IMDS-first and override fallback
  behaviours so the ≥95% statement coverage floor remains intact (§§5, 11).
- Mode A/Mode B sample configs adopt less aggressive controller targets (0.20–0.32),
  smaller step adjustments, slower estimator cadence, and only two worker threads
  so production deployments consume less CPU while staying near the documented
  thresholds. The manifests also drop hard-coded OCIDs and rely on IMDS by default
  (§§3, 5).
- Rootless Mode A manifests, runtime script, and docs now restore the `SHAPER_CPU_SHARES` default to `128`, reflecting that rootless
  Docker honours delegated cgroup v2 CPU weight overrides (§6).
- Rootless Mode A Compose samples now include the plan-aligned `# cpus: ${SHAPER_CPUS:-0.30}` line plus matching environment
  variable and documentation guidance so operators can uncomment the stanza, set `SHAPER_CPUS`, and confirm `docker compose config`
  renders the expected quota before deployment (§§6, 12).
- Refreshed `docs/00-overview.md` to document the current CLI flag surface, configuration layout, and navigation map, including forthcoming quick-start and CLI references (§§0, 5, 9).
- Extended `docs/00-overview.md` with the plan-required threat model and non-goals sections and replaced the placeholder Quick Start note with a link to the published §10 onboarding guide so operators can navigate the consolidated deployment references (§§0, 10, 12).
- Clarified the documentation roadmap to mark the published CLI/deployment guides and onboarding workflows as complete while calibrating remaining milestones for future adaptive-controller and release updates (§12).
- CLI now starts the metrics HTTP server using `http.bind`/`HTTP_ADDR`, shuts it down with the run context, and ships container/Compose updates (`EXPOSE 9108`, `${SHAPER_METRICS_BIND}`) so `/metrics` is reachable when enabled; the listener now logs structured bind/serve failures and returns an explicit shutdown hook so docs can describe the exporter lifecycle and monitoring workflow alignment (§§6, 9, 11).
- CLI metadata resolution now populates `oci.compartmentId`/`OCI_COMPARTMENT_ID` alongside the new `oci.region`/`OCI_REGION` overrides using IMDS when online, threads the resolved region into the Monitoring client, and logs both identifiers for observability. Fresh unit coverage in §11 exercises the success, fallback, and error paths so the ≥95% statement floor holds.
- Runtime metadata resolution now prefers the canonical region name exposed by IMDS and
  only falls back to the legacy `instance/region` endpoint when that lookup is missing or
  fails. Monitoring clients therefore receive the long-form OCI region identifiers even on
  regions that still return short codes, and the updated CLI tests cover both the canonical
  and fallback flows to maintain the ≥95% coverage floor (§§2, 5, 11).
- CLI now installs `SIGINT`/`SIGTERM` handlers that cancel the run context so the controller, worker pool, and metrics HTTP server exit gracefully. The new `tests/e2e/signal_shutdown_test.go` suite delivers both signals to the binary and asserts the structured shutdown logs to keep §11’s coverage contract intact (§§5, 9, 11).
- IMDS client now injects the required IMDSv2 authorisation header and exposes canonical-region plus compartment OCID lookups, with unit tests and docs refreshed to keep §2 aligned with the metadata surface.
- Canonical-region lookups now read the `regionInfo` block returned by `/instance/`, aligning the IMDS client, CLI emulation server, and documentation with the current OCI metadata layout (§2).
- CLI `--mode` handling now starts the adaptive controller in `dry-run`/`enforce`, keeps `noop` as a diagnostics bypass, and logs configuration failures surfaced by the new YAML/environment loader. Updated docs in §§5 and 9 describe the operating modes and tunable configuration.
- Raised the CI statement coverage floor to 95% and filtered `make coverage` to exclude developer tooling packages (for example, `cmd/agentscheck`), bringing the latest production-only run to 95.1% while keeping the threshold focused on shipped code paths (§11).
- CLI argument parsing now validates supported controller modes and normalises flag input before wiring placeholder subsystems.
- §11 development workflow now mandates shipping changes only after `go test ./... -race` and `golangci-lint run` succeed, reinforcing the all-tests-pass requirement alongside the existing ≥95% coverage guardrail.
- CLI runtime configuration accepts an `oci.instanceId`/`OCI_INSTANCE_ID` override so dry-run and enforce modes can bootstrap when IMDS access is unavailable (e.g., CI smoke tests), with docs refreshed in §§2 and 9.
- CLI runtime configuration now recognises `oci.offline`/`OCI_OFFLINE`, substituting a static metrics client and fallback instance ID so dry-run and enforce bootstrap without IMDS or Monitoring access. Container docs in §§8 and 9 cover the new smoke-test defaults.
- Logger construction returns actionable errors for invalid levels while keeping structured output defaults consistent.
- Container build now targets the latest Go toolchain and documentation references the up-to-date requirements.
- Raised the module `go` directive, `.tool-versions`, and container build ARG to Go 1.25.4 so CI, local workflows, and release images all consume the latest patched toolchain required by `govulncheck` (§14).
- CI and release automation now leverage GitHub Actions caching to speed linting, testing, and multi-architecture builds, including restoring the runner `~/.cache/go-build` directory alongside module downloads (§14).
- Container smoke testing now runs the packaged binary with `--log-level debug --shutdown-after=4s`, verifies the offline metadata log and graceful shutdown message, and uses a tighter offline configuration so CI fails quickly when wiring regresses (§§8, 9, 11).
- Release SBOM generation is pinned to the latest Anchore Syft GitHub Action for up-to-date SPDX output (§14).
- Release workflow now builds and pushes both distroless `rootless` and rootful image targets with dedicated tags plus per-variant SBOM artifacts so operators can pull the UID profile their deployment requires (§§6, 14).
- Refreshed §§4–9 documentation to describe the Prometheus endpoint, offline/static metrics client behaviour, and updated `QueryP95CPU` interface so operators see the current exporter wiring and Monitoring contract.
- Local lint tooling is standardised on `golangci-lint` v2.6.1 with pinned installation in CI and the developer Makefile helper, keeping contributor environments aligned (§14).
- `make lint`/`make test` now create repository-local caches (`.cache/golangci` and `.cache/go`) and set `GOLANGCI_LINT_CACHE`/`GOCACHE` accordingly so the tools never write to protected runner directories; prefer using the Makefile helpers instead of invoking the linters or `go test` manually to keep sandbox runs stable (§14).
- `.tool-versions` now pins `golangci-lint` v2.6.1 so `mise`/`asdf` environments surface the same linting behaviour developers see in CI (§14).
- `golangci-lint` now runs with depguard allow-listing for module imports and `issues.fix: true`, letting formatters auto-apply fixes while docs instruct contributors to stage the generated edits (§14).
- Overview, README, and Monitoring documentation now link to the IAM, reclaim, cgroup, alarm, and Quick Start guides so operators can navigate the consolidated Always Free playbook (§§0, 5, 10).
- Updated third-party Go modules (flock, gobreaker, testify, golang.org/x/{crypto,net,sys}) to their latest releases so the controller wiring, samplers, and tests stay aligned with upstream fixes (§§11, 14).
- OCI Go SDK bumped to v65.105.0 and `go.uber.org/zap` to v1.27.1 so the logging stack and Monitoring client stay current with upstream bugfixes and SDK updates (§§11, 14).
- Reconfirmed all Go module requirements and GitHub Actions pins are on the latest stable releases, updating workflow actions to their freshest tags to keep CI and release automation current (§§11, 14).
- Added a `make bench` helper that wraps `hack/check_benchmarks.sh` and wired the same duty-cycle benchmark enforcement into the CI workflow so every pull request runs the CPU/fairness regression gate alongside linting and tests (§§11, 14).
