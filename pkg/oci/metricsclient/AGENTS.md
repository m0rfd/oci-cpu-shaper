# AGENTS

## Scope: `pkg/oci/metricsclient/`
- Keep this package focused on building/wrapping OCI Monitoring clients; default builders should stay thin adapters over `pkg/oci` constructors.
- Context helpers here own metrics builder injection; avoid reintroducing global seams in `cmd/shaper`.
- Keep tests split by concern (builder/context vs. wrapper behaviour) and short to match the surrounding guidance.
