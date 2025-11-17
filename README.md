# OCI CPU Shaper

OCI CPU Shaper is an adaptive controller for shaping CPU utilization of workloads running on Oracle Cloud Infrastructure. The fully implemented controller now ships in the CLI and Compose/Quadlet bundles, so operators can run dry-run or enforce modes with live OCI metrics today instead of waiting for future milestones. New operators should begin with the [Quick Start](docs/10-quick-start.md) to complete the mandatory console setup before exploring the reference material.

## Repository Structure

- `cmd/shaper/` – Entry point for the CLI binary with `app` wiring that applies CPU shaping logic.
- `pkg/` – Shared packages divided into domains for metadata (`imds`), OCI integrations (`oci`), estimation (`est`), shaping algorithms (`shape`), adaptation (`adapt`), and HTTP helpers (`http`).
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

Contributions are welcome! Please:

1. Open an issue to discuss significant features or changes.
2. Follow Go best practices and the formatting rules defined in `.editorconfig`.
3. Use the provided tooling shortcuts before submitting changes and keep the ≥95% statement coverage guarantee in place:
   - `make fmt` to format code with `go fmt`.
   - `make lint` to run `golangci-lint` with the cached configuration described in the docs.
   - `make test` to execute the suite with the Go race detector enabled.
   - `make coverage MIN_COVERAGE=95` to confirm the repository-wide coverage threshold documented in §11 of the implementation plan.
  - `make integration` to verify Docker connectivity, ensure the cgroup v2 CPU controller is present, build the distroless rootful and nonroot images, and run the CPU weight responsiveness tests with logs mirrored to `artifacts/integration.log`.
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

Images published to `ghcr.io/<owner>/oci-cpu-shaper` are signed with Cosign using GitHub Actions’ OIDC keyless flow, and the workflow uploads detached signatures plus SBOM attestations as release assets for both `nonroot` and `rootful` variants. Download the files that match the release tag you are deploying and verify either the image or the Syft-generated SPDX attestation:

```bash
TAG="v1.2.3"
VARIANT="nonroot" # or rootful
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
