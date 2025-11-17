# OCI CPU Shaper

OCI CPU Shaper is an adaptive controller for shaping CPU utilization of workloads running on Oracle Cloud Infrastructure. The fully implemented controller now ships in the CLI and Compose/Quadlet bundles, so operators can run dry-run or enforce modes with live OCI metrics today instead of waiting for future milestones. New operators should begin with the [Quick Start](docs/10-quick-start.md) to complete the mandatory console setup before exploring the reference material.

## Repository Structure

- `cmd/shaper/` – Entry point for the CLI binary that applies CPU shaping logic.
- `pkg/` – Shared packages divided into domains for metadata (`imds`), OCI integrations (`oci`), estimation (`est`), shaping algorithms (`shape`), adaptation (`adapt`), and HTTP helpers (`http`).
- `internal/buildinfo/` – Build metadata embedded into binaries.
- `configs/` – Example configuration files and templates, including `mode-a.yaml`
  and `mode-b.yaml` which ship the documented defaults referenced in
  [`docs/09-cli.md`](docs/09-cli.md).
- `deploy/` – Deployment manifests and automation assets.
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
   - `make integration` to verify Docker connectivity, enforce cgroup v2, and run the CPU weight responsiveness tests with logs mirrored to `artifacts/integration.log`.
   - `make build` to ensure binaries compile successfully.
4. Include tests and documentation updates when adding new functionality.
5. Use conventional commit messages where possible to ease changelog generation.

See [`docs/08-development.md`](docs/08-development.md) for detailed local development setup guidance.

Refer to the documentation in the `docs/` directory for deeper architectural and operational context as it becomes available.
