# OCI CPU Shaper

[![Go 1.25.x](https://img.shields.io/badge/Go-1.25.x-00ADD8?logo=go)](go.mod)
[![OCI VM Ready](https://img.shields.io/badge/OCI%20VM-ready-fa6400?logo=oracle)](docs/10-quick-start.md)
[![Cosign Releases](https://img.shields.io/badge/Releases-Cosign%20signed-0f9d58?logo=cosign)](#release-verification)

- **Supported runtimes:** Docker/Podman Compose Mode A (rootless) and Mode B (rootful) Quadlet manifests that deploy the
  published distroless containers onto Oracle Cloud VMs via [`deploy/`](deploy/).
- **OCI tenancy requirements:** Instance Monitoring plugin enabled, a Dynamic Group plus tenancy policy that permits Monitoring access, and the seven-day `CpuUtilization` alarm sequence outlined in [§10 Quick Start](docs/10-quick-start.md).

OCI CPU Shaper is an adaptive controller for shaping CPU utilization of workloads running on Oracle Cloud Infrastructure. The fully implemented controller now ships in the CLI and Compose/Quadlet bundles, so operators can run dry-run or enforce modes with live OCI metrics today instead of waiting for future milestones. New operators should begin with the [Quick Start](docs/10-quick-start.md) to complete the mandatory console setup before exploring the reference material.

## Architecture

The shaper stitches together IMDSv2 metadata, tenancy-wide Monitoring queries, the adaptive controller, worker pools, and the Prometheus HTTP surfaces described throughout the docs set. Review the [architecture diagram](docs/00-overview.md#architecture-diagram) for the canonical layout before diving into the deeper sections on policies, controller flows, and exported metrics.

## Getting Started

Run the container release that matches your OCI VM architecture and follow the linked docs before wiring IAM policies:

```bash
TAG="v1.2.3"             # pin to the release you trust
VARIANT="rootless"        # or rootful for Mode B
IMAGE="ghcr.io/<owner>/oci-cpu-shaper:${TAG}-${VARIANT}"

docker pull "$IMAGE"
docker image inspect "$IMAGE" | jq '.[0].Config.Labels'
```

```bash
# Render the provided Compose bundle locally
cp deploy/compose/mode-a.rootless.yaml ./docker-compose.yaml
TAG="v1.2.3" VARIANT="rootless" envsubst < docker-compose.yaml > docker-compose.rendered.yaml
docker compose -f docker-compose.rendered.yaml up -d
```

- Start with the baked-in [`configs/mode-a.yaml`](configs/mode-a.yaml) or [`configs/mode-b.yaml`](configs/mode-b.yaml) manifests and override values in place or by bind-mounting host files as shown in [`deploy/compose/`](deploy/compose/).
- Follow the five onboarding moves in [§10 Quick Start Onboarding](docs/10-quick-start.md) to enable Monitoring metrics, IAM policies, and alarms before applying enforce mode.
- Review [`docs/09-cli.md`](docs/09-cli.md) for container environment overrides, health endpoints, and `/metrics` expectations before exposing the service to Prometheus.

## Feature Highlights

| Area | Highlights |
| --- | --- |
| Mode A (rootless) | Ships as the default Compose stack with `cpu.weight` 128, non-root distroless images, and loopback metrics publishing so Oracle Cloud VM operators can stay within managed-host guardrails. |
| Mode B (rootful) | Adds `SYS_NICE` for optional `SCHED_IDLE`, host networking, and Quadlet units so privileged tuning can be evaluated without rewriting manifests when deeper host control is required. |
| Metrics endpoint | The container exposes Prometheus metrics on `:9108` by default, covering controller state, OCI P95 samples, and cgroup discoveries for parity checks with the VM’s own telemetry. |
| Release verification | Distroless container images and SBOM attestations are signed with Cosign; detached signatures + certificates accompany every GitHub release for offline validation prior to pulling onto an OCI VM. |

## Repository Structure

- `cmd/shaper/` – CLI entry point split across `app.go`/`app_run.go` for signal-safe wiring, controller factories in `controller_helpers.go`, flag parsing in `options.go`, runtime metadata logging in `logging_metadata.go`, and metrics wiring in `metrics_handlers.go`/`metrics_server.go`.
- `pkg/runtimeconfig/` – Shared runtime configuration defaults (`defaults.go`), YAML/env loaders (`loader.go`, `merge.go`, `env.go`), validation (`validate.go`), and helpers that translate manifests into adaptive-controller structs consumed by the CLI.
- `pkg/adapt/` – Adaptive controller and mode handling kept in `controller_loop.go`, `controller_step.go`, `controller_state.go`, `suppression.go`, `config.go`, and the dry-run adapter in `noop_controller.go`.
- `pkg/shape/` – Worker pool, pause thresholds, and busy-wait helpers implemented in `pool.go`, `worker.go`, `pause.go`, and `busywait.go` (plus the rootful-only stubs/tests).
- `pkg/oci/` – OCI Monitoring client wiring (`client.go`), metric queries (`query.go`), SDK adapter (`sdk_client.go`), and offline fixtures (`static.go`).
- `pkg/imds/` – Instance Metadata Service client split into constructor wiring (`client_config.go`), typed operations (`operations.go`), and HTTP/retry helpers (`transport.go`).
- `pkg/est/`, `pkg/http/`, and `pkg/cgroup/` – Supporting packages for load estimation, Prometheus exporters, and cgroup discovery.
- `internal/buildinfo/` – Build metadata embedded into binaries.
- `configs/` – Example configuration files and templates, including `mode-a.yaml`
  and `mode-b.yaml` which ship the documented defaults referenced in
  [`docs/09-cli.md`](docs/09-cli.md).
- `deploy/` – Deployment manifests and automation assets. Compose bundles such as
  `deploy/compose/mode-a.rootless.yaml` now expose the optional `SHAPER_CPUS`
  knob documented in §6, so operators can uncomment the provided `cpus:` stanza,
  set a fractional value in `deploy/compose/mode-a.env.example`, and run
  `docker compose --file deploy/compose/mode-a.rootless.yaml config` to verify
  the rendered CPU cap before deploying.
- `docs/` – Living documentation; begin with [`00-overview.md`](docs/00-overview.md).

## Contribution Guidelines

See [CONTRIBUTING.md](CONTRIBUTING.md) for the complete tooling workflow,
documentation expectations, and scoped `AGENTS.md` policy. Contributions are
welcome! Please:

1. Open an issue to discuss significant features or changes.
2. Follow Go best practices and the formatting rules defined in `.editorconfig`.
3. Use the provided tooling shortcuts before submitting changes and keep the ≥98% statement coverage guarantee in place:
   - `make setup` (fresh Ubuntu 24.x container) to install Go, base build tools, module dependencies, linting helpers, and the `pre-commit` hook that runs `make lint`. Ensure your `PATH` includes `/usr/local/go/bin` and `$HOME/go/bin` (or your `GOBIN`) so the installed tooling is discoverable.
   - `make maintenance` (resumed container) to refresh Go modules and tooling without reinstalling the toolchain.
   - `make lint-fix` to run `golangci-lint` with autofix enabled.
   - `make lint` to run checks only.
   - `make test` to execute the suite with the Go race detector enabled. Set `RUN_E2E_TESTS=1 make test` when you need the tagged CLI end-to-end harness that exercises the mock IMDS/Monitoring servers in `tests/e2e`. The environment flag stays unset in untrusted contexts (for example, pull requests from forks) so the suite only runs when explicitly requested.
   - `make coverage MIN_COVERAGE=98` to confirm the repository-wide coverage threshold documented in §11 of the implementation plan. The target now merges unit, integration, and optional tagged E2E profiles with `gocovmerge` so `coverage.out`/`coverage.txt` always reflect the combined results; use `RUN_E2E_TESTS=1 KEEP_E2E_COVERAGE=1 make coverage` when you need the E2E profile preserved as `coverage-e2e.out` for reuse in CI.
   - `make check` to run linting, tests, coverage enforcement, CodeQL, and agent verification in one pass. Set `CHECK_INCLUDE_CODEQL=0 make check` if you want to mirror CI’s faster path where CodeQL runs in its dedicated workflow.
   - `make codeql-setup` to install the CodeQL CLI into a versioned toolcache directory, retain existing installs, and prefetch the default query packs into `.cache/codeql/packs` for offline analysis. `make codeql`, `make codeql-actions`, `make codeql-go`, or `make codeql-all` mirror the PR CodeQL checks directly, writing databases to `.cache/codeql/databases`, emitting SARIF artifacts to `artifacts/codeql/`, and honoring `CODEQL_DATABASE_ROOT`, `CODEQL_CACHE_DIR`, and `CODEQL_PACK_CACHE_DIR` overrides. Go analysis uses the baked-in build command (no extra manual steps), runs `codeql database analyze` with `--threads=0`, and you can override the query suites with `CODEQL_ACTIONS_QUERY_PACK`/`CODEQL_GO_QUERY_PACK` when experimenting locally. Use `make codeql-clean` to prune generated databases and SARIF outputs without disturbing the cached CLI or packs.
   - `make integration` to verify Docker connectivity, ensure the cgroup v2 CPU controller is present, build the distroless rootful and rootless images, and run the CPU weight responsiveness tests with logs mirrored to `artifacts/integration.log`.
   - `make build` to ensure binaries compile successfully.
4. Include tests and documentation updates when adding new functionality.
5. Use conventional commit messages where possible to ease changelog generation.

See [`docs/08-development.md`](docs/08-development.md) for detailed local development setup guidance.

## Validating Mode B SCHED_IDLE support

Mode B deployments rely on the `SYS_NICE` capability to enter Linux `SCHED_IDLE`
when the worker pool starts. Operators can validate a host before rolling out a
rootful deployment by running the integration suite and inspecting the logs for
the `worker failed to enter sched_idle` warning:

```
make integration INTEGRATION_KEEP_LOGS=1
```

The new `TestSchedIdleWarningTracksSysNiceCapability` case builds the binary
with `-tags rootful`, launches a distroless Mode B container twice, and runs it
as the `nobody` user (which lacks Linux capabilities) before rerunning as
`root` with `SYS_NICE` explicitly granted. The warning only appears when the
capability is absent, and the container logs are mirrored to
`artifacts/integration.log` whenever `INTEGRATION_KEEP_LOGS=1`, giving operators
a quick signal that the host kernel honours the `SCHED_IDLE` downgrade before
promoting the change to production.

Refer to the documentation in the `docs/` directory for deeper architectural and operational context as it becomes available.

## Release Verification

Images published to `ghcr.io/<owner>/oci-cpu-shaper` are signed with Cosign using GitHub Actions’ OIDC keyless flow, and the workflow uploads detached signatures plus SBOM attestations as release assets for both `rootless` and `rootful` variants. Download the files that match the release tag you are deploying and verify either the image or the Syft-generated SPDX attestation:

The workflow pins Cosign to `v3.0.2` via `sigstore/cosign-installer` so the signing and verification flags below stay stable across releases; adjust the pin when upgrading Cosign.

```bash
TAG="v1.2.3"
VARIANT="rootless" # or rootful
IMAGE="ghcr.io/<owner>/oci-cpu-shaper:${VARIANT}"

cosign verify \
  --certificate-identity "https://github.com/<owner>/oci-cpu-shaper/.github/workflows/release.yml@refs/tags/${TAG}" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  --signature cosign-${TAG}-${VARIANT}.sig \
  --certificate cosign-${TAG}-${VARIANT}.pem \
  "$IMAGE"

cosign verify-attestation \
  --type spdx \
  --certificate-identity "https://github.com/<owner>/oci-cpu-shaper/.github/workflows/release.yml@refs/tags/${TAG}" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  --signature sbom-attestation-${TAG}-${VARIANT}.sig \
  --certificate sbom-attestation-${TAG}-${VARIANT}.pem \
  --attestation sbom-attestation-${TAG}-${VARIANT}.jsonl \
  "$IMAGE"
```

For additional context and troubleshooting guidance, see the §14 signing section in [`docs/08-development.md`](docs/08-development.md).
